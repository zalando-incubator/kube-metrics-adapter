package nakadi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQuery(tt *testing.T) {
	client := &http.Client{}
	for _, ti := range []struct {
		msg                string
		status             int
		responseBody       string
		err                error
		unconsumedEvents   int64
		consumerLagSeconds int64
	}{
		{
			msg:    "test getting a single event-type",
			status: http.StatusOK,
			responseBody: `{
					  "items": [
					    {
					      "event_type": "example-event",
					      "partitions": [
						{
						  "partition": "0",
						  "state": "assigned",
						  "unconsumed_events": 4,
						  "consumer_lag_seconds": 2,
						  "stream_id": "example-id",
						  "assignment_type": "auto"
						},
						{
						  "partition": "0",
						  "state": "assigned",
						  "unconsumed_events": 5,
						  "consumer_lag_seconds": 1,
						  "stream_id": "example-id",
						  "assignment_type": "auto"
						}
					     ]
					   }
					 ]
				       }`,
			unconsumedEvents:   9,
			consumerLagSeconds: 2,
		},
		{
			msg:    "test getting multiple event-types",
			status: http.StatusOK,
			responseBody: `{
					  "items": [
					    {
					      "event_type": "example-event",
					      "partitions": [
						{
						  "partition": "0",
						  "state": "assigned",
						  "unconsumed_events": 4,
						  "consumer_lag_seconds": 2,
						  "stream_id": "example-id",
						  "assignment_type": "auto"
						},
						{
						  "partition": "0",
						  "state": "assigned",
						  "unconsumed_events": 5,
						  "consumer_lag_seconds": 1,
						  "stream_id": "example-id",
						  "assignment_type": "auto"
						}
					      ]
					     },
					     {
					      "event_type": "example-event-2",
					      "partitions": [
						{
						  "partition": "0",
						  "state": "assigned",
						  "unconsumed_events": 4,
						  "consumer_lag_seconds": 6,
						  "stream_id": "example-id",
						  "assignment_type": "auto"
						},
						{
						  "partition": "0",
						  "state": "assigned",
						  "unconsumed_events": 5,
						  "consumer_lag_seconds": 1,
						  "stream_id": "example-id",
						  "assignment_type": "auto"
						}
					       ]
					     }
					  ]
				       }`,
			unconsumedEvents:   18,
			consumerLagSeconds: 6,
		},
		{
			msg:          "test call with invalid response",
			status:       http.StatusInternalServerError,
			responseBody: `{"error": 500}`,
			err:          errors.New("[nakadi stats] unexpected response code: 500 ({\"error\": 500})"),
		},
		{
			msg:    "test getting back a single data point",
			status: http.StatusOK,
			responseBody: `{
					  "items": []
				       }`,
			err: errors.New("expected at least 1 event-type, 0 returned"),
		},
	} {
		tt.Run(ti.msg, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(ti.status)
					_, err := w.Write([]byte(ti.responseBody))
					assert.NoError(t, err)
				}),
			)
			defer ts.Close()

			nakadiClient := NewNakadiClient(ts.URL, client)
			consumerLagSeconds, err := nakadiClient.ConsumerLagSeconds(context.Background(), "id")
			assert.Equal(t, ti.err, err)
			assert.Equal(t, ti.consumerLagSeconds, consumerLagSeconds)
			unconsumedEvents, err := nakadiClient.UnconsumedEvents(context.Background(), "id")
			assert.Equal(t, ti.err, err)
			assert.Equal(t, ti.unconsumedEvents, unconsumedEvents)
		})
	}

}
