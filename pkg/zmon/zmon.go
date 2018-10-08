package zmon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	// set of valid aggregators that can be used in queries
	validAggregators = map[string]struct{}{
		"avg":   struct{}{},
		"dev":   struct{}{},
		"count": struct{}{},
		"first": struct{}{},
		"last":  struct{}{},
		"max":   struct{}{},
		"min":   struct{}{},
		"sum":   struct{}{},
		"diff":  struct{}{},
	}
)

// Entity defines a ZMON entity.
type Entity struct {
	ID string `json:"id"`
}

// ZMON defines an interface for talking to the ZMON API.
type ZMON interface {
	Entities(filter map[string]string) ([]Entity, error)
	Query(checkID int, key string, entities, aggregators []string, duration time.Duration) ([]DataPoint, error)
}

// Client defines client for interfacing with the ZMON API.
type Client struct {
	zmonEndpoint        string
	dataServiceEndpoint string
	http                *http.Client
}

// NewZMONClient initializes a new ZMON Client.
func NewZMONClient(zmonEndpoint, dataServiceEndpoint string, client *http.Client) *Client {
	return &Client{
		zmonEndpoint:        zmonEndpoint,
		dataServiceEndpoint: dataServiceEndpoint,
		http:                client,
	}
}

// Entities returns a list of entities based on the passed filter.
func (c *Client) Entities(filter map[string]string) ([]Entity, error) {
	endpoint, err := url.Parse(c.zmonEndpoint)
	if err != nil {
		return nil, err
	}

	filterData, err := json.Marshal(&filter)
	if err != nil {
		return nil, err
	}

	q := endpoint.Query()
	q.Set("query", string(filterData))
	endpoint.RawQuery = q.Encode()
	endpoint.Path = "/api/v1/entities"

	req, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	d, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected response code: %d", resp.StatusCode)
	}

	var entities []Entity
	err = json.Unmarshal(d, &entities)
	if err != nil {
		return nil, err
	}

	return entities, nil
}

// DataPoint defines a single datapoint returned from a query.
type DataPoint struct {
	Time  time.Time
	Value float64
}

type metricQuery struct {
	StartAbsolute int64    `json:"start_absolute"`
	StartRelative sampling `json:"start_relative"`
	Metrics       []metric `json:"metrics"`
}

type sampling struct {
	Value int64  `json:"value"`
	Unit  string `json:"unit"`
}

type metric struct {
	Name        string       `json:"name"`
	Limit       int          `json:"limit"`
	Tags        tags         `json:"tags"`
	GroupBy     []tagGroup   `json:"group_by"`
	Aggregators []aggregator `json:"aggregator"`
}

type tags struct {
	Key    []string `json:"key,omitempty"`
	Entity []string `json:"entity,omitempty"`
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

// Query queries the ZMON KariosDB endpoint and returns the resulting list of
// data points for the query.
//
// https://kairosdb.github.io/docs/build/html/restapi/QueryMetrics.html
func (c *Client) Query(checkID int, key string, entities, aggregators []string, duration time.Duration) ([]DataPoint, error) {
	endpoint, err := url.Parse(c.dataServiceEndpoint)
	if err != nil {
		return nil, err
	}

	startTime := time.Now().UTC().Add(-duration)

	query := metricQuery{
		StartAbsolute: startTime.UnixNano() / 1000,
		Metrics: []metric{
			{
				Name:  fmt.Sprintf("zmon.check.%d", checkID),
				Limit: 10000, // maximum limit of ZMON
				Tags: tags{
					Entity: entities,
				},
				GroupBy: []tagGroup{
					{
						Name: "tag",
						Tags: []string{
							"entity",
							"key",
						},
					},
				},
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
		query.Metrics[0].Tags.Key = []string{key}
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

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	d, err := ioutil.ReadAll(resp.Body)
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

// KairosDBEntityFormat converts an entity id to the kairosDB compatible
// format.
func KairosDBEntityFormat(id string) string {
	e := strings.Replace(id, "[", "_", -1)
	e = strings.Replace(e, "]", "_", -1)
	e = strings.Replace(e, ":", "_", -1)
	return e
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
