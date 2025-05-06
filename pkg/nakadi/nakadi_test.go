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

	subscriptionsResponseBody := `{
  "items": [
    {
      "id": "id_1"
    },
    {
      "id": "id_2"
    }
  ],
  "_links": {
    "next": {
      "href": "/subscriptions?event_type=example-event&owning_application=example-app&offset=20&limit=20"
    }
  }
}`

	subscriptionsResponseBodyNoNext := `{
  "items": [],
  "_links": {}
}`

	for _, ti := range []struct {
		msg                        string
		status                     int
		subscriptionIDResponseBody string
		subscriptionFilter         *SubscriptionFilter
		err                        error
		unconsumedEvents           int64
		consumerLagSeconds         int64
	}{
		{
			msg:                "test getting a single event-type",
			status:             http.StatusOK,
			subscriptionFilter: &SubscriptionFilter{SubscriptionID: "id"},
			subscriptionIDResponseBody: `{
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
			msg:                "test getting multiple event-types",
			status:             http.StatusOK,
			subscriptionFilter: &SubscriptionFilter{SubscriptionID: "id"},
			subscriptionIDResponseBody: `{
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
			msg:                        "test call with invalid response",
			status:                     http.StatusInternalServerError,
			subscriptionFilter:         &SubscriptionFilter{SubscriptionID: "id"},
			subscriptionIDResponseBody: `{"error": 500}`,
			err:                        errors.New("[nakadi stats] unexpected response code: 500 ({\"error\": 500})"),
		},
		{
			msg:                "test getting back no data points",
			status:             http.StatusOK,
			subscriptionFilter: &SubscriptionFilter{SubscriptionID: "id"},
			subscriptionIDResponseBody: `{
					  "items": []
				       }`,
			err: errors.New("[nakadi stats] expected at least 1 event-type, 0 returned"),
		},
		{
			msg:                "test filtering by owning_application and event_type",
			status:             http.StatusOK,
			subscriptionFilter: &SubscriptionFilter{OwningApplication: "example-app", EventTypes: []string{"example-event"}, ConsumerGroup: "example-group"},
			subscriptionIDResponseBody: `{
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
			unconsumedEvents:   18,
			consumerLagSeconds: 2,
		},
	} {
		tt.Run(ti.msg, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("/subscriptions", func(w http.ResponseWriter, r *http.Request) {
				offset := r.URL.Query().Get("offset")
				if offset != "" {
					w.WriteHeader(http.StatusOK)
					_, err := w.Write([]byte(subscriptionsResponseBodyNoNext))
					assert.NoError(t, err)
					return
				}

				owningApplication := r.URL.Query().Get("owning_application")
				eventTypes := r.URL.Query()["event_type"]
				consumerGroup := r.URL.Query().Get("consumer_group")

				assert.Equal(t, ti.subscriptionFilter.OwningApplication, owningApplication)
				assert.Equal(t, ti.subscriptionFilter.EventTypes, eventTypes)
				assert.Equal(t, ti.subscriptionFilter.ConsumerGroup, consumerGroup)

				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte(subscriptionsResponseBody))
				assert.NoError(t, err)
			})
			mux.HandleFunc("/subscriptions/{id}/stats", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(ti.status)
				_, err := w.Write([]byte(ti.subscriptionIDResponseBody))
				assert.NoError(t, err)
			})
			ts := httptest.NewServer(mux)
			defer ts.Close()

			nakadiClient := NewNakadiClient(ts.URL, client)
			consumerLagSeconds, err := nakadiClient.ConsumerLagSeconds(context.Background(), ti.subscriptionFilter)
			assert.Equal(t, ti.err, err)
			assert.Equal(t, ti.consumerLagSeconds, consumerLagSeconds)
			unconsumedEvents, err := nakadiClient.UnconsumedEvents(context.Background(), ti.subscriptionFilter)
			assert.Equal(t, ti.err, err)
			assert.Equal(t, ti.unconsumedEvents, unconsumedEvents)
		})
	}
}
