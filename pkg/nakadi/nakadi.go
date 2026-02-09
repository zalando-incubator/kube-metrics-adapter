package nakadi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/cenkalti/backoff/v5"
)

// Nakadi defines an interface for talking to the Nakadi API.
type Nakadi interface {
	ConsumerLagSeconds(ctx context.Context, filter *SubscriptionFilter) (int64, error)
	UnconsumedEvents(ctx context.Context, filter *SubscriptionFilter) (int64, error)
}

// Client defines client for interfacing with the Nakadi API.
type Client struct {
	nakadiEndpoint string
	http           *http.Client
}

// NewNakadiClient initializes a new Nakadi Client.
func NewNakadiClient(nakadiEndpoint string, client *http.Client) *Client {
	return &Client{
		nakadiEndpoint: nakadiEndpoint,
		http:           client,
	}
}

func (c *Client) ConsumerLagSeconds(ctx context.Context, filter *SubscriptionFilter) (int64, error) {
	stats, err := c.stats(ctx, filter)
	if err != nil {
		return 0, err
	}

	var maxConsumerLagSeconds int64
	for _, eventType := range stats {
		for _, partition := range eventType.Partitions {
			maxConsumerLagSeconds = max(maxConsumerLagSeconds, partition.ConsumerLagSeconds)
		}
	}

	return maxConsumerLagSeconds, nil
}

func (c *Client) UnconsumedEvents(ctx context.Context, filter *SubscriptionFilter) (int64, error) {
	stats, err := c.stats(ctx, filter)
	if err != nil {
		return 0, err
	}

	var unconsumedEvents int64
	for _, eventType := range stats {
		for _, partition := range eventType.Partitions {
			unconsumedEvents += partition.UnconsumedEvents
		}
	}

	return unconsumedEvents, nil
}

type SubscriptionFilter struct {
	SubscriptionID    string
	OwningApplication string
	EventTypes        []string
	ConsumerGroup     string
}

func (c *Client) subscriptions(ctx context.Context, filter *SubscriptionFilter, href string) ([]string, error) {
	endpoint, err := url.Parse(c.nakadiEndpoint)
	if err != nil {
		return nil, err
	}

	if href != "" {
		endpoint, err = url.Parse(c.nakadiEndpoint + href)
		if err != nil {
			return nil, fmt.Errorf("[nakadi subscriptions] failed to parse URL with href: %w", err)
		}
	} else {
		endpoint.Path = "/subscriptions"
		q := endpoint.Query()
		if filter.OwningApplication != "" {
			q.Set("owning_application", filter.OwningApplication)
		}
		for _, eventType := range filter.EventTypes {
			q.Add("event_type", eventType)
		}
		if filter.ConsumerGroup != "" {
			q.Set("consumer_group", filter.ConsumerGroup)
		}
		endpoint.RawQuery = q.Encode()
	}

	op := func() ([]byte, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("[nakadi subscriptions] failed to create request: %w", err)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("[nakadi subscriptions] failed to make request: %w", err)
		}
		defer resp.Body.Close()

		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		if err := checkResponseStatus(resp, b); err != nil {
			return nil, fmt.Errorf("[nakadi subscriptions] %w", err)
		}

		return b, nil
	}

	d, err := backoff.Retry(
		ctx,
		op,
		backoff.WithBackOff(exponentialBackoff()),
		backoff.WithMaxTries(3),
	)
	if err != nil {
		return nil, err
	}

	var subscriptionsResp struct {
		Items []struct {
			ID string `json:"id"`
		}
		Links struct {
			Next struct {
				Href string `json:"href"`
			} `json:"next"`
		} `json:"_links"`
	}
	err = json.Unmarshal(d, &subscriptionsResp)
	if err != nil {
		return nil, err
	}

	var subscriptions []string
	for _, item := range subscriptionsResp.Items {
		subscriptions = append(subscriptions, item.ID)
	}

	if subscriptionsResp.Links.Next.Href != "" {
		nextSubscriptions, err := c.subscriptions(ctx, nil, subscriptionsResp.Links.Next.Href)
		if err != nil {
			return nil, fmt.Errorf("[nakadi subscriptions] failed to get next subscriptions: %w", err)
		}
		subscriptions = append(subscriptions, nextSubscriptions...)
	}

	return subscriptions, nil
}

type statsResp struct {
	Items []statsEventType `json:"items"`
}

type statsEventType struct {
	EventType  string           `json:"event_type"`
	Partitions []statsPartition `json:"partitions"`
}

type statsPartition struct {
	Partiton           string `json:"partition"`
	State              string `json:"state"`
	UnconsumedEvents   int64  `json:"unconsumed_events"`
	ConsumerLagSeconds int64  `json:"consumer_lag_seconds"`
	StreamID           string `json:"stream_id"`
	AssignmentType     string `json:"assignment_type"`
}

// stats returns the Nakadi stats for a given a subscription filter which can
// include the subscription ID or a filter combination of [owning-applicaiton,
// event-types, consumer-group]..
//
// https://nakadi.io/manual.html#/subscriptions/subscription_id/stats_get
func (c *Client) stats(ctx context.Context, filter *SubscriptionFilter) ([]statsEventType, error) {
	var subscriptionIDs []string
	if filter.SubscriptionID == "" {
		subscriptions, err := c.subscriptions(ctx, filter, "")
		if err != nil {
			return nil, fmt.Errorf("[nakadi stats] failed to get subscriptions: %w", err)
		}
		subscriptionIDs = subscriptions
	} else {
		subscriptionIDs = []string{filter.SubscriptionID}
	}

	endpoint, err := url.Parse(c.nakadiEndpoint)
	if err != nil {
		return nil, fmt.Errorf("[nakadi stats] failed to parse URL %q: %w", c.nakadiEndpoint, err)
	}

	var stats []statsEventType
	for _, subscriptionID := range subscriptionIDs {
		endpoint.Path = fmt.Sprintf("/subscriptions/%s/stats", subscriptionID)

		q := endpoint.Query()
		q.Set("show_time_lag", "true")
		endpoint.RawQuery = q.Encode()

		op := func() ([]byte, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
			if err != nil {
				return nil, fmt.Errorf("[nakadi stats] failed to create request: %w", err)
			}

			resp, err := c.http.Do(req)
			if err != nil {
				return nil, fmt.Errorf("[nakadi stats] failed to make request: %w", err)
			}
			defer resp.Body.Close()

			b, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, err
			}

			if err := checkResponseStatus(resp, b); err != nil {
				return nil, fmt.Errorf("[nakadi stats] %w", err)
			}

			return b, nil
		}

		d, err := backoff.Retry(
			ctx,
			op,
			backoff.WithBackOff(exponentialBackoff()),
			backoff.WithMaxTries(3),
		)
		if err != nil {
			return nil, err
		}

		var result statsResp
		err = json.Unmarshal(d, &result)
		if err != nil {
			return nil, err
		}

		if len(result.Items) == 0 {
			return nil, errors.New("[nakadi stats] expected at least 1 event-type, 0 returned")
		}

		stats = append(stats, result.Items...)
	}

	return stats, nil
}

func checkResponseStatus(resp *http.Response, b []byte) error {
	if resp.StatusCode == http.StatusTooManyRequests {
		h := resp.Header.Get("Retry-After")
		if h == "" {
			return fmt.Errorf("unexpected response code: %d (%s)", resp.StatusCode, string(b))

		}
		sec, err := strconv.ParseInt(h, 10, 32)
		if err != nil {
			return backoff.Permanent(err)
		}
		return backoff.RetryAfter(int(sec))
	}
	if resp.StatusCode >= http.StatusBadRequest &&
		resp.StatusCode < http.StatusInternalServerError {
		return backoff.Permanent(
			fmt.Errorf("non-retryable response code: %d (%s)", resp.StatusCode, string(b)),
		)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response code: %d (%s)", resp.StatusCode, string(b))
	}
	return nil
}

func exponentialBackoff() backoff.BackOff {
	b := backoff.NewExponentialBackOff()
	b.MaxInterval = time.Second * 30
	b.InitialInterval = time.Millisecond * 100
	return b
}
