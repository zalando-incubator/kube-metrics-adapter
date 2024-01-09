package nakadi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Nakadi defines an interface for talking to the Nakadi API.
type Nakadi interface {
	ConsumerLagSeconds(ctx context.Context, subscriptionID string) (int64, error)
	UnconsumedEvents(ctx context.Context, subscriptionID string) (int64, error)
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

func (c *Client) ConsumerLagSeconds(ctx context.Context, subscriptionID string) (int64, error) {
	stats, err := c.stats(ctx, subscriptionID)
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

func (c *Client) UnconsumedEvents(ctx context.Context, subscriptionID string) (int64, error) {
	stats, err := c.stats(ctx, subscriptionID)
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

// stats returns the Nakadi stats for a given subscription ID.
//
// https://nakadi.io/manual.html#/subscriptions/subscription_id/stats_get
func (c *Client) stats(ctx context.Context, subscriptionID string) ([]statsEventType, error) {
	endpoint, err := url.Parse(c.nakadiEndpoint)
	if err != nil {
		return nil, err
	}

	endpoint.Path = fmt.Sprintf("/subscriptions/%s/stats", subscriptionID)

	q := endpoint.Query()
	q.Set("show_time_lag", "true")
	endpoint.RawQuery = q.Encode()

	resp, err := c.http.Get(endpoint.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	d, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("[nakadi stats] unexpected response code: %d (%s)", resp.StatusCode, string(d))
	}

	var result statsResp
	err = json.Unmarshal(d, &result)
	if err != nil {
		return nil, err
	}

	if len(result.Items) == 0 {
		return nil, errors.New("expected at least 1 event-type, 0 returned")
	}

	return result.Items, nil
}
