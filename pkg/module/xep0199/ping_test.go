// Copyright 2022 The jackal Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package xep0199

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	kitlog "github.com/go-kit/log"

	"github.com/jackal-xmpp/stravaganza"
	streamerror "github.com/jackal-xmpp/stravaganza/errors/stream"
	"github.com/jackal-xmpp/stravaganza/jid"
	"github.com/ortuman/jackal/pkg/hook"
	"github.com/ortuman/jackal/pkg/router"
	"github.com/ortuman/jackal/pkg/router/stream"
	"github.com/stretchr/testify/require"
)

func TestPing_Pong(t *testing.T) {
	// given
	outBuf := bytes.NewBuffer(nil)

	routerMock := &routerMock{}
	routerMock.RouteFunc = func(ctx context.Context, stanza stravaganza.Stanza) ([]jid.JID, error) {
		_ = stanza.ToXML(outBuf, true)
		return nil, nil
	}
	p := New(Config{}, routerMock, &hook.Hooks{}, kitlog.NewNopLogger())

	// when
	iq, _ := stravaganza.NewIQBuilder().
		WithAttribute(stravaganza.ID, "s2s1").
		WithAttribute(stravaganza.Type, stravaganza.GetType).
		WithAttribute(stravaganza.From, "jabber.org").
		WithAttribute(stravaganza.To, "jackal.im").
		WithChild(
			stravaganza.NewBuilder("ping").
				WithAttribute(stravaganza.Namespace, "urn:xmpp:ping").
				Build(),
		).
		BuildIQ()
	_ = p.ProcessIQ(context.Background(), iq)

	// then
	require.Equal(t, `<iq id='s2s1' type='result' from='jackal.im' to='jabber.org'/>`, outBuf.String())
}

func TestPing_SendPing(t *testing.T) {
	// given
	var mu sync.Mutex
	var outStanza stravaganza.Stanza

	routerMock := &routerMock{}
	routerMock.RouteFunc = func(ctx context.Context, stanza stravaganza.Stanza) ([]jid.JID, error) {
		mu.Lock()
		defer mu.Unlock()
		outStanza = stanza
		return nil, nil
	}
	hk := hook.NewHooks()
	p := New(Config{
		Interval:  time.Millisecond * 500,
		SendPings: true,
	}, routerMock, hk, kitlog.NewNopLogger())
	jd, _ := jid.NewWithString("ortuman@jackal.im/yard", true)

	// when
	_ = p.Start(context.Background())
	_, _ = hk.Run(context.Background(), hook.C2SStreamBinded, &hook.ExecutionContext{
		Info: &hook.C2SStreamInfo{
			ID:  "c2s1",
			JID: jd,
		},
	})
	time.Sleep(time.Second) // wait until ping is triggered

	// then
	mu.Lock()
	defer mu.Unlock()

	require.NotNil(t, outStanza)
	require.Equal(t, stravaganza.GetType, outStanza.Attribute(stravaganza.Type))
	require.NotNil(t, outStanza.ChildNamespace("ping", pingNamespace))
}

func TestPing_Timeout(t *testing.T) {
	// given
	routerMock := &routerMock{}
	routerMock.RouteFunc = func(ctx context.Context, stanza stravaganza.Stanza) ([]jid.JID, error) {
		return nil, nil
	}
	c2sStream := &streamMock{}
	c2sStream.DisconnectFunc = func(streamErr *streamerror.Error) <-chan error {
		return nil
	}
	c2sRouterMock := &c2sRouterMock{}
	c2sRouterMock.LocalStreamFunc = func(username string, resource string) stream.C2S {
		return c2sStream
	}
	routerMock.C2SFunc = func() router.C2SRouter {
		return c2sRouterMock
	}

	hk := hook.NewHooks()
	p := New(Config{
		Interval:      time.Millisecond * 500,
		AckTimeout:    time.Millisecond * 250,
		SendPings:     true,
		TimeoutAction: killAction,
	}, routerMock, hk, kitlog.NewNopLogger())
	jd, _ := jid.NewWithString("ortuman@jackal.im/yard", true)

	// when
	_ = p.Start(context.Background())
	_, _ = hk.Run(context.Background(), hook.C2SStreamBinded, &hook.ExecutionContext{
		Info: &hook.C2SStreamInfo{
			ID:  "c2s1",
			JID: jd,
		},
	})
	time.Sleep(time.Second) // wait until ping is triggered

	// then
	require.Len(t, c2sStream.DisconnectCalls(), 1)
}
