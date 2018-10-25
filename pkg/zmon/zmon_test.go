package zmon

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestQuery(tt *testing.T) {
	client := &http.Client{}
	for _, ti := range []struct {
		msg         string
		duration    time.Duration
		aggregators []string
		status      int
		body        string
		err         error
		dataPoints  []DataPoint
		key         string
	}{
		{
			msg:      "test getting back a single data point",
			duration: 1 * time.Hour,
			status:   http.StatusOK,
			body: `{
			         "queries": [
				   {
				     "results": [
				       {
					 "values": [
					  [1539710395000,765952]
					]
				       }
				     ]
				   }
				 ]
			}`,
			dataPoints: []DataPoint{
				{
					Time:  time.Unix(1539710395, 0),
					Value: 765952,
				},
			},
		},
		{
			msg:      "test getting back a single datapoint with key",
			duration: 1 * time.Hour,
			status:   http.StatusOK,
			key:      "my-key",
			body: `{
			         "queries": [
				   {
				     "results": [
				       {
					 "values": [
					  [1539710395000,765952]
					]
				       }
				     ]
				   }
				 ]
			}`,
			dataPoints: []DataPoint{
				{
					Time:  time.Unix(1539710395, 0),
					Value: 765952,
				},
			},
		},
		{
			msg:         "test getting back a single datapoint with aggregators",
			duration:    1 * time.Hour,
			status:      http.StatusOK,
			aggregators: []string{"max"},
			body: `{
			         "queries": [
				   {
				     "results": [
				       {
					 "values": [
					  [1539710395000,765952]
					]
				       }
				     ]
				   }
				 ]
			}`,
			dataPoints: []DataPoint{
				{
					Time:  time.Unix(1539710395, 0),
					Value: 765952,
				},
			},
		},
		{
			msg:         "test query with invalid aggregator",
			aggregators: []string{"invalid"},
			err:         fmt.Errorf("invalid aggregator 'invalid'"),
		},
		{
			msg:    "test query with invalid response",
			status: http.StatusInternalServerError,
			body:   `{"error": 500}`,
			err:    fmt.Errorf("[kariosdb query] unexpected response code: 500"),
		},
		{
			msg:      "test getting invalid values response",
			duration: 1 * time.Hour,
			status:   http.StatusOK,
			body: `{
			         "queries": [
				   {
				     "results": [
				       {
					 "values": [
					  [1539710395000,765952,1]
					]
				       }
				     ]
				   }
				 ]
			}`,
			err: fmt.Errorf("[kariosdb query] unexpected response data"),
		},
	} {
		tt.Run(ti.msg, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(ti.status)
					w.Write([]byte(ti.body))
				}),
			)
			defer ts.Close()

			zmonClient := NewZMONClient(ts.URL, client)
			dataPoints, err := zmonClient.Query(1, ti.key, nil, ti.aggregators, ti.duration)
			assert.Equal(t, ti.err, err)
			assert.Len(t, dataPoints, len(ti.dataPoints))
			assert.Equal(t, ti.dataPoints, dataPoints)
		})
	}

}

func TestKariosDBEntityFormat(t *testing.T) {
	entity := "aws:1234567:eu-central-1:kube-1[pod-1]"
	assert.Equal(t, "aws_1234567_eu-central-1_kube-1_pod-1_", KairosDBEntityFormat(entity))
}

func TestDurationToSampling(tt *testing.T) {
	for _, ti := range []struct {
		msg      string
		duration time.Duration
		sampling sampling
	}{
		{
			msg:      "1 hour should map to hours sampling",
			duration: 1 * time.Hour,
			sampling: sampling{
				Unit:  "hours",
				Value: 1,
			},
		},
		{
			msg:      "2 years should map to years sampling",
			duration: 2 * day * 365,
			sampling: sampling{
				Unit:  "years",
				Value: 2,
			},
		},
		{
			msg:      "1 nanosecond should map to 0 milliseconds sampling",
			duration: 1,
			sampling: sampling{
				Unit:  "milliseconds",
				Value: 0,
			},
		},
	} {
		tt.Run(ti.msg, func(t *testing.T) {
			assert.Equal(t, durationToSampling(ti.duration), ti.sampling)
		})
	}
}
