package zmon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

var (
	// set of valid aggregators that can be used in queries
	// https://kairosdb.github.io/docs/build/html/restapi/Aggregators.html
	validAggregators = map[string]struct{}{
		"avg":   {},
		"count": {},
		"last":  {},
		"max":   {},
		"min":   {},
		"sum":   {},
		"diff":  {},
	}
)

// Entity defines a ZMON entity.
type Entity struct {
	ID string `json:"id"`
}

// ZMON defines an interface for talking to the ZMON API.
type ZMON interface {
	Query(checkID int, key string, tags map[string]string, aggregators []string, duration time.Duration) ([]DataPoint, error)
}

// Client defines client for interfacing with the ZMON API.
type Client struct {
	dataServiceEndpoint string
	http                *http.Client
}

// NewZMONClient initializes a new ZMON Client.
func NewZMONClient(dataServiceEndpoint string, client *http.Client) *Client {
	return &Client{
		dataServiceEndpoint: dataServiceEndpoint,
		http:                client,
	}
}

// DataPoint defines a single datapoint returned from a query.
type DataPoint struct {
	Time  time.Time
	Value float64
}

type metricQuery struct {
	StartRelative sampling `json:"start_relative"`
	Metrics       []metric `json:"metrics"`
}

type sampling struct {
	Value int64  `json:"value"`
	Unit  string `json:"unit"`
}

type metric struct {
	Name        string              `json:"name"`
	Limit       int                 `json:"limit"`
	Tags        map[string][]string `json:"tags"`
	GroupBy     []tagGroup          `json:"group_by"`
	Aggregators []aggregator        `json:"aggregators"`
}

type tagGroup struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

type aggregator struct {
	Name     string   `json:"name"`
	Sampling sampling `json:"sampling"`
}

type queryResp struct {
	Queries []struct {
		Results []struct {
			Values [][]float64 `json:"values"`
		} `json:"results"`
	} `json:"queries"`
}

// Query queries the ZMON KairosDB endpoint and returns the resulting list of
// data points for the query.
//
// https://kairosdb.github.io/docs/build/html/restapi/QueryMetrics.html
func (c *Client) Query(checkID int, key string, tags map[string]string, aggregators []string, duration time.Duration) ([]DataPoint, error) {
	endpoint, err := url.Parse(c.dataServiceEndpoint)
	if err != nil {
		return nil, err
	}

	// convert tags map
	tagsSlice := make(map[string][]string, len(tags))
	for k, v := range tags {
		tagsSlice[k] = []string{v}
	}

	query := metricQuery{
		StartRelative: durationToSampling(duration),
		Metrics: []metric{
			{
				Name:        fmt.Sprintf("zmon.check.%d", checkID),
				Limit:       10000, // maximum limit of ZMON
				Tags:        tagsSlice,
				GroupBy:     []tagGroup{},
				Aggregators: make([]aggregator, 0, len(aggregators)),
			},
		},
	}

	// add aggregators
	for _, aggregatorName := range aggregators {
		if _, ok := validAggregators[aggregatorName]; !ok {
			return nil, fmt.Errorf("invalid aggregator '%s'", aggregatorName)
		}
		query.Metrics[0].Aggregators = append(query.Metrics[0].Aggregators, aggregator{
			Name:     aggregatorName,
			Sampling: durationToSampling(duration),
		})
	}

	// add key to query if defined
	if key != "" {
		query.Metrics[0].Tags["key"] = []string{key}
		query.Metrics[0].GroupBy = append(query.Metrics[0].GroupBy, tagGroup{
			Name: "tag",
			Tags: []string{"key"},
		})
	}

	body, err := json.Marshal(&query)
	if err != nil {
		return nil, err
	}

	endpoint.Path += "/api/v1/datapoints/query"

	req, err := http.NewRequest(http.MethodPost, endpoint.String(), bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Attribution", fmt.Sprintf("kube-metrics-adapter/%d", checkID))

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	d, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("[kariosdb query] unexpected response code: %d", resp.StatusCode)
	}

	var result queryResp
	err = json.Unmarshal(d, &result)
	if err != nil {
		return nil, err
	}

	if len(result.Queries) < 1 {
		return nil, nil
	}

	if len(result.Queries[0].Results) < 1 {
		return nil, nil
	}

	dataPoints := make([]DataPoint, 0, len(result.Queries[0].Results[0].Values))
	for _, value := range result.Queries[0].Results[0].Values {
		if len(value) != 2 {
			return nil, fmt.Errorf("[kariosdb query] unexpected response data")
		}
		point := DataPoint{
			Time:  time.Unix(0, int64(value[0])*1000000),
			Value: value[1],
		}
		dataPoints = append(dataPoints, point)
	}

	return dataPoints, nil
}

const (
	day   = 24 * time.Hour
	week  = day * 7
	month = day * 30
	year  = day * 365
)

// durationToSampling converts a time.Duration to the sampling format expected
// by karios db. E.g. the duration `1 * time.Hour` would be converted to:
// sampling{
//   Unit: "minutes",
//   Value: 1,
// }
func durationToSampling(d time.Duration) sampling {
	for _, u := range []struct {
		Unit        string
		Nanoseconds time.Duration
	}{
		{
			Unit:        "years",
			Nanoseconds: year,
		},
		{
			Unit:        "months",
			Nanoseconds: month,
		},
		{
			Unit:        "weeks",
			Nanoseconds: week,
		},
		{
			Unit:        "days",
			Nanoseconds: day,
		},
		{
			Unit:        "hours",
			Nanoseconds: 1 * time.Hour,
		},
		{
			Unit:        "minutes",
			Nanoseconds: 1 * time.Minute,
		},
		{
			Unit:        "seconds",
			Nanoseconds: 1 * time.Second,
		},
		{
			Unit:        "milliseconds",
			Nanoseconds: 1 * time.Millisecond,
		},
	} {
		if d.Nanoseconds()/int64(u.Nanoseconds) >= 1 {
			return sampling{
				Unit:  u.Unit,
				Value: int64(d.Round(u.Nanoseconds) / u.Nanoseconds),
			}
		}
	}

	return sampling{
		Unit:  "milliseconds",
		Value: 0,
	}
}
