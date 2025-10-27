package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/require"
	"github.com/thesicktwist1/harmony/shared"
)

func testclients(server *server) map[string]*Client {
	return map[string]*Client{
		"test_client_1": newClient(nil, server),
		"test_client_2": newClient(nil, server),
		"test_client_3": newClient(nil, server),
	}
}

func makeMsg(eventType shared.EnvelopeType, event shared.FileEvent) ([]byte, error) {
	msg, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(shared.Envelope{
		Type:    eventType,
		Message: msg,
	})
	if err != nil {
		return nil, err
	}
	return data, nil
}

func TestServerResponses(t *testing.T) {
	var (
		ctx    = context.Background()
		server = NewServer(ctx, nil)
	)
	tests := []struct {
		name         string
		serverFunc   func([]byte, *Client)
		wantReceived map[string]struct{}
		message      func() []byte
		sender       string
	}{
		{
			name:       "server broadcast (message from test_client_3)",
			serverFunc: server.broadcast,
			message: func() []byte {
				msg, err := makeMsg(
					shared.Event,
					shared.FileEvent{
						Path: "path",
						Op:   fsnotify.Create.String(),
					},
				)
				require.NoError(t, err)
				return msg
			},
			wantReceived: map[string]struct{}{"test_client_2": {}, "test_client_1": {}},
			sender:       "test_client_3",
		},
		{
			name:       "server respond (message from test_client_2)",
			serverFunc: server.respond,
			message: func() []byte {
				msg, err := makeMsg(
					shared.Event,
					shared.FileEvent{
						Path: "path",
						Op:   shared.Update,
					},
				)
				require.NoError(t, err)
				return msg
			},
			wantReceived: map[string]struct{}{"test_client_2": {}},
			sender:       "test_client_2",
		},
	}
	for _, tc := range tests {
		clients := testclients(server)
		for name, c := range clients {
			c.name = name
			server.addClient(c)
		}
		sender, exist := clients[tc.sender]
		require.True(t, exist)

		tc.serverFunc(tc.message(), sender)

		clientmsgs := map[string][]byte{}
		for name, c := range clients {
			select {
			case clientmsgs[name] = <-c.msgBuffer:

			default:
			}
		}
		for name := range tc.wantReceived {
			got, exist := clientmsgs[name]
			require.True(t, exist)
			require.Equal(t, tc.message(), got)
		}
		require.Equal(t, len(tc.wantReceived), len(clientmsgs))
	}
}
