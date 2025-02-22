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

package xep0198

import (
	"context"
	"math/rand"
	"testing"
	"time"

	clusterconnmanager "github.com/ortuman/jackal/pkg/cluster/connmanager"

	"github.com/ortuman/jackal/pkg/cluster/instance"

	kitlog "github.com/go-kit/log"
	"github.com/google/uuid"
	"github.com/jackal-xmpp/stravaganza"
	streamerror "github.com/jackal-xmpp/stravaganza/errors/stream"
	"github.com/jackal-xmpp/stravaganza/jid"
	"github.com/ortuman/jackal/pkg/hook"
	c2smodel "github.com/ortuman/jackal/pkg/model/c2s"
	streamqueue "github.com/ortuman/jackal/pkg/module/xep0198/queue"
	"github.com/ortuman/jackal/pkg/router/stream"
	xmpputil "github.com/ortuman/jackal/pkg/util/xmpp"
	"github.com/stretchr/testify/require"
)

func TestStream_EncodeSMID(t *testing.T) {
	// given
	jd, _ := jid.NewWithString("ortuman@jackal.im/yard", true)

	nonce := make([]byte, nonceLength)
	for i := range nonce {
		nonce[i] = byte(i + 1)
	}
	// when
	smID := encodeSMID(jd, nonce)

	// then
	require.Equal(t, "b3J0dW1hbkBqYWNrYWwuaW0veWFyZAABAgMEBQYHCAkKCwwNDg8QERITFBUWFxg=", smID)
}

func TestStream_DecodeSMID(t *testing.T) {
	// given
	smID := "b3J0dW1hbkBqYWNrYWwuaW0vQ29udmVyc2F0aW9ucy40UllFAHl5Jrx+gnSZ7hq3vjoW38oQM2ZrPknCyA=="

	// when
	jd, nonce, err := decodeSMID(smID)

	// then
	require.Nil(t, err)
	require.NotNil(t, jd)
	require.NotNil(t, nonce)

	expectedNonce := []byte{
		0x79, 0x79, 0x26, 0xbc, 0x7e, 0x82, 0x74, 0x99,
		0xee, 0x1a, 0xb7, 0xbe, 0x3a, 0x16, 0xdf, 0xca,
		0x10, 0x33, 0x66, 0x6b, 0x3e, 0x49, 0xc2, 0xc8,
	}
	require.Equal(t, "ortuman@jackal.im/Conversations.4RYE", jd.String())
	require.Equal(t, expectedNonce, nonce)
}

func TestStream_Enable(t *testing.T) {
	// given
	jd, _ := jid.NewWithString("ortuman@jackal.im/yard", true)

	stmMock := &c2sStreamMock{}

	var setK string
	var setVal interface{}
	stmMock.IDFunc = func() stream.C2SID { return 1234 }
	stmMock.JIDFunc = func() *jid.JID { return jd }
	stmMock.UsernameFunc = func() string { return jd.Node() }
	stmMock.ResourceFunc = func() string { return jd.Resource() }
	stmMock.SetInfoValueFunc = func(ctx context.Context, k string, val interface{}) error {
		setK = k
		setVal = val
		return nil
	}
	stmMock.IsBindedFunc = func() bool { return true }
	stmMock.InfoFunc = func() c2smodel.Info { return c2smodel.NewInfoMap() }

	var sentEl stravaganza.Element
	stmMock.SendElementFunc = func(elem stravaganza.Element) <-chan error {
		sentEl = elem
		return nil
	}

	hk := hook.NewHooks()
	sm := &Stream{
		cfg:         testSMConfig(),
		stmQueueMap: streamqueue.NewQueueMap(),
		hk:          hk,
		logger:      kitlog.NewNopLogger(),
	}

	// when
	_ = sm.Start(context.Background())
	defer func() { _ = sm.Stop(context.Background()) }()

	halted, err := hk.Run(context.Background(), hook.C2SStreamElementReceived, &hook.ExecutionContext{
		Info: &hook.C2SStreamInfo{
			Element: stravaganza.NewBuilder("enable").
				WithAttribute(stravaganza.Namespace, streamNamespace).
				Build(),
		},
		Sender: stmMock,
	})

	// then
	require.True(t, halted)
	require.Nil(t, err)

	require.Equal(t, setK, enabledInfoKey)
	require.Equal(t, true, setVal)

	require.Equal(t, "enabled", sentEl.Name())
	require.Equal(t, streamNamespace, sentEl.Attribute(stravaganza.Namespace))

	sq := sm.stmQueueMap.Get(queueKey(jd))
	require.NotNil(t, sq)

	sq.CancelTimers()
}

func TestStream_InStanza(t *testing.T) {
	// given
	jd, _ := jid.NewWithString("ortuman@jackal.im/yard", true)

	stmMock := &c2sStreamMock{}
	stmMock.JIDFunc = func() *jid.JID { return jd }
	stmMock.InfoFunc = func() c2smodel.Info {
		return c2smodel.NewInfoMapFromMap(
			map[string]string{enabledInfoKey: "true"},
		)
	}

	hk := hook.NewHooks()
	sm := &Stream{
		cfg:         testSMConfig(),
		stmQueueMap: streamqueue.NewQueueMap(),
		hk:          hk,
		logger:      kitlog.NewNopLogger(),
	}
	sq := streamqueue.New(
		stmMock, nil, nil, 0, 0, time.Second, time.Minute,
	)
	sm.stmQueueMap.Set(queueKey(jd), sq)

	sq.CancelTimers() // do not send R
	defer sq.CancelTimers()

	b := stravaganza.NewMessageBuilder()
	b.WithAttribute("from", "noelia@jackal.im/yard")
	b.WithAttribute("to", "ortuman@jackal.im/yard")
	b.WithChild(
		stravaganza.NewBuilder("body").
			WithText("I'll give thee a wind.").
			Build(),
	)
	testMsg, _ := b.BuildMessage()

	// when
	_ = sm.Start(context.Background())
	defer func() { _ = sm.Stop(context.Background()) }()

	_, err := hk.Run(context.Background(), hook.C2SStreamElementReceived, &hook.ExecutionContext{
		Info:   &hook.C2SStreamInfo{Element: testMsg},
		Sender: stmMock,
	})

	// then
	require.Nil(t, err)

	sq = sm.stmQueueMap.Get(queueKey(jd))
	require.NotNil(t, sq)

	require.Equal(t, uint32(1), sq.InboundH())
}

func TestStream_OutStanza(t *testing.T) {
	// given
	jd, _ := jid.NewWithString("ortuman@jackal.im/yard", true)

	stmMock := &c2sStreamMock{}
	stmMock.JIDFunc = func() *jid.JID { return jd }
	stmMock.InfoFunc = func() c2smodel.Info {
		return c2smodel.NewInfoMapFromMap(
			map[string]string{enabledInfoKey: "true"},
		)
	}

	hk := hook.NewHooks()
	sm := &Stream{
		cfg:         testSMConfig(),
		stmQueueMap: streamqueue.NewQueueMap(),
		hk:          hk,
		logger:      kitlog.NewNopLogger(),
	}
	sq := streamqueue.New(
		stmMock, nil, nil, 0, 0, time.Second, time.Minute,
	)
	sm.stmQueueMap.Set(queueKey(jd), sq)

	sq.CancelTimers() // do not send R
	defer sq.CancelTimers()

	b := stravaganza.NewMessageBuilder()
	b.WithAttribute("from", "ortuman@jackal.im/yard")
	b.WithAttribute("to", "noelia@jackal.im/yard")
	b.WithChild(
		stravaganza.NewBuilder("body").
			WithText("I'll give thee a wind.").
			Build(),
	)
	testMsg, _ := b.BuildMessage()

	// when
	_ = sm.Start(context.Background())
	defer func() { _ = sm.Stop(context.Background()) }()

	_, err := hk.Run(context.Background(), hook.C2SStreamElementSent, &hook.ExecutionContext{
		Info:   &hook.C2SStreamInfo{Element: testMsg},
		Sender: stmMock,
	})

	// then
	require.Nil(t, err)

	sq = sm.stmQueueMap.Get(queueKey(jd))
	require.NotNil(t, sq)

	require.Len(t, sq.Elements(), 1)
	require.Equal(t, sq.Elements()[0].Stanza, testMsg)
	require.Equal(t, uint32(1), sq.Elements()[0].H)
}

func TestStream_OutStanzaMaxQueueSizeReached(t *testing.T) {
	// given
	jd, _ := jid.NewWithString("ortuman@jackal.im/yard", true)

	stmMock := &c2sStreamMock{}
	stmMock.IDFunc = func() stream.C2SID { return 1234 }
	stmMock.JIDFunc = func() *jid.JID { return jd }
	stmMock.UsernameFunc = func() string { return jd.Node() }
	stmMock.ResourceFunc = func() string { return jd.Resource() }
	stmMock.InfoFunc = func() c2smodel.Info {
		return c2smodel.NewInfoMapFromMap(
			map[string]string{enabledInfoKey: "true"},
		)
	}
	var streamErr *streamerror.Error
	stmMock.DisconnectFunc = func(sErr *streamerror.Error) <-chan error {
		streamErr = sErr
		return nil
	}

	cfg := testSMConfig()
	cfg.MaxQueueSize = 1

	hk := hook.NewHooks()
	sm := &Stream{
		cfg:         cfg,
		stmQueueMap: streamqueue.NewQueueMap(),
		hk:          hk,
		logger:      kitlog.NewNopLogger(),
	}
	b := stravaganza.NewMessageBuilder()
	b.WithAttribute("from", "ortuman@jackal.im/yard")
	b.WithAttribute("to", "noelia@jackal.im/yard")
	b.WithChild(
		stravaganza.NewBuilder("body").
			WithText("I'll give thee a wind.").
			Build(),
	)
	testMsg1, _ := b.BuildMessage()
	testMsg2, _ := b.BuildMessage()

	sq := streamqueue.New(
		stmMock, nil, nil, 0, 0, time.Second, time.Minute,
	)
	sq.HandleOut(testMsg1)

	sm.stmQueueMap.Set(queueKey(jd), sq)

	sq.CancelTimers() // do not send R
	defer sq.CancelTimers()

	// when
	_ = sm.Start(context.Background())
	defer func() { _ = sm.Stop(context.Background()) }()

	_, err := hk.Run(context.Background(), hook.C2SStreamElementSent, &hook.ExecutionContext{
		Info:   &hook.C2SStreamInfo{Element: testMsg2},
		Sender: stmMock,
	})

	// then
	require.Nil(t, err)

	require.NotNil(t, streamErr)
	require.Equal(t, streamerror.PolicyViolation, streamErr.Reason)
}

func TestStream_SendR(t *testing.T) {
	// given
	jd, _ := jid.NewWithString("ortuman@jackal.im/yard", true)

	stmMock := &c2sStreamMock{}
	stmMock.IDFunc = func() stream.C2SID { return 1234 }
	stmMock.JIDFunc = func() *jid.JID { return jd }
	stmMock.UsernameFunc = func() string { return jd.Node() }
	stmMock.ResourceFunc = func() string { return jd.Resource() }
	stmMock.InfoFunc = func() c2smodel.Info {
		return c2smodel.NewInfoMapFromMap(
			map[string]string{enabledInfoKey: "true"},
		)
	}
	sendCh := make(chan stravaganza.Element, 1)
	stmMock.SendElementFunc = func(elem stravaganza.Element) <-chan error {
		sendCh <- elem
		return nil
	}

	hk := hook.NewHooks()
	sm := &Stream{
		cfg:         testSMConfig(),
		stmQueueMap: streamqueue.NewQueueMap(),
		hk:          hk,
		logger:      kitlog.NewNopLogger(),
	}
	sq := streamqueue.New(
		stmMock, nil, nil, 0, 0, time.Millisecond*500, time.Minute,
	)
	sm.stmQueueMap.Set(queueKey(jd), sq)
	defer sq.CancelTimers()

	// when
	var sentEl stravaganza.Element
	select {
	case el := <-sendCh:
		sentEl = el

	case <-time.After(time.Second * 5):
		require.Fail(t, "Failed to receive R element")
		return
	}

	// then
	require.NotNil(t, sentEl)

	require.Equal(t, "r", sentEl.Name())
	require.Equal(t, streamNamespace, sentEl.Attribute(stravaganza.Namespace))
}

func TestStream_HandleR(t *testing.T) {
	// given
	jd, _ := jid.NewWithString("ortuman@jackal.im/yard", true)

	stmMock := &c2sStreamMock{}
	stmMock.IDFunc = func() stream.C2SID { return 1234 }
	stmMock.JIDFunc = func() *jid.JID { return jd }
	stmMock.UsernameFunc = func() string { return jd.Node() }
	stmMock.ResourceFunc = func() string { return jd.Resource() }
	stmMock.InfoFunc = func() c2smodel.Info {
		return c2smodel.NewInfoMapFromMap(
			map[string]string{enabledInfoKey: "true"},
		)
	}
	var sentEl stravaganza.Element
	stmMock.SendElementFunc = func(elem stravaganza.Element) <-chan error {
		sentEl = elem
		return nil
	}

	hk := hook.NewHooks()
	sm := &Stream{
		cfg:         testSMConfig(),
		stmQueueMap: streamqueue.NewQueueMap(),
		hk:          hk,
		logger:      kitlog.NewNopLogger(),
	}
	sq := streamqueue.New(
		stmMock, nil, nil, 10, 0, time.Second, time.Minute,
	)
	sm.stmQueueMap.Set(queueKey(jd), sq)

	sq.CancelTimers() // do not send R
	defer sq.CancelTimers()

	// when
	_ = sm.Start(context.Background())
	defer func() { _ = sm.Stop(context.Background()) }()

	halted, err := hk.Run(context.Background(), hook.C2SStreamElementReceived, &hook.ExecutionContext{
		Info: &hook.C2SStreamInfo{
			Element: stravaganza.NewBuilder("r").
				WithAttribute(stravaganza.Namespace, streamNamespace).
				Build(),
		},
		Sender: stmMock,
	})

	// then
	require.True(t, halted)
	require.Nil(t, err)

	require.Equal(t, "a", sentEl.Name())
	require.Equal(t, "10", sentEl.Attribute("h"))
}

func TestStream_HandleA(t *testing.T) {
	// given
	jd, _ := jid.NewWithString("ortuman@jackal.im/yard", true)

	stmMock := &c2sStreamMock{}
	stmMock.IDFunc = func() stream.C2SID { return 1234 }
	stmMock.JIDFunc = func() *jid.JID { return jd }
	stmMock.UsernameFunc = func() string { return jd.Node() }
	stmMock.ResourceFunc = func() string { return jd.Resource() }
	stmMock.InfoFunc = func() c2smodel.Info {
		return c2smodel.NewInfoMapFromMap(
			map[string]string{enabledInfoKey: "true"},
		)
	}
	var sentEl stravaganza.Element
	stmMock.SendElementFunc = func(elem stravaganza.Element) <-chan error {
		sentEl = elem
		return nil
	}

	b := stravaganza.NewMessageBuilder()
	b.WithAttribute("from", "ortuman@jackal.im/yard")
	b.WithAttribute("to", "noelia@jackal.im/yard")
	b.WithChild(
		stravaganza.NewBuilder("body").
			WithText("I'll give thee a wind.").
			Build(),
	)
	uid := uuid.New().String()
	b.WithAttribute("id", uid)
	testMsg1, _ := b.BuildMessage()

	uid2 := uuid.New().String()
	b.WithAttribute("id", uid2)
	testMsg2, _ := b.BuildMessage()

	uid3 := uuid.New().String()
	b.WithAttribute("id", uid3)
	testMsg3, _ := b.BuildMessage()

	elements := []streamqueue.Element{
		{Stanza: testMsg1, H: 20},
		{Stanza: testMsg2, H: 21},
		{Stanza: testMsg3, H: 22},
	}

	hk := hook.NewHooks()
	sm := &Stream{
		cfg:         testSMConfig(),
		stmQueueMap: streamqueue.NewQueueMap(),
		hk:          hk,
		logger:      kitlog.NewNopLogger(),
	}
	sq := streamqueue.New(
		stmMock, nil, elements, 0, 0, time.Second, time.Minute,
	)
	sm.stmQueueMap.Set(queueKey(jd), sq)

	sq.CancelTimers() // do not send R
	defer sq.CancelTimers()

	// when
	_ = sm.Start(context.Background())
	defer func() { _ = sm.Stop(context.Background()) }()

	halted, err := hk.Run(context.Background(), hook.C2SStreamElementReceived, &hook.ExecutionContext{
		Info: &hook.C2SStreamInfo{
			Element: stravaganza.NewBuilder("a").
				WithAttribute(stravaganza.Namespace, streamNamespace).
				WithAttribute("h", "21").
				Build(),
		},
		Sender: stmMock,
	})

	// then
	require.True(t, halted)
	require.Nil(t, err)

	sq = sm.stmQueueMap.Get(queueKey(jd))
	require.NotNil(t, sq)
	require.Len(t, sq.Elements(), 1)

	require.NotNil(t, sentEl)
	require.Equal(t, uid3, sentEl.Attribute("id"))
}

func TestStream_Resume(t *testing.T) {
	// given
	jd, _ := jid.NewWithString("ortuman@jackal.im/yard", true)

	stmMock := &c2sStreamMock{}
	stmMock.IsAuthenticatedFunc = func() bool { return true }
	stmMock.IDFunc = func() stream.C2SID { return 1234 }
	stmMock.JIDFunc = func() *jid.JID { return jd }
	stmMock.UsernameFunc = func() string { return jd.Node() }
	stmMock.ResourceFunc = func() string { return jd.Resource() }
	stmMock.DisconnectFunc = func(_ *streamerror.Error) <-chan error { return nil }

	sndElements := make([]stravaganza.Element, 0)
	stmMock.SendElementFunc = func(elem stravaganza.Element) <-chan error {
		sndElements = append(sndElements, elem)
		return nil
	}
	var resumed bool
	stmMock.ResumeFunc = func(ctx context.Context, jd *jid.JID, pr *stravaganza.Presence, inf c2smodel.Info) error {
		resumed = true
		return nil
	}

	resMngMock := &resourceManagerMock{}
	resMngMock.GetResourceFunc = func(ctx context.Context, username string, resource string) (c2smodel.ResourceDesc, error) {
		return c2smodel.NewResourceDesc(
			instance.ID(),
			jd,
			xmpputil.MakePresence(jd, jd.ToBareJID(), stravaganza.AvailableType, nil),
			c2smodel.NewInfoMapFromMap(
				map[string]string{enabledInfoKey: "true"},
			),
		), nil
	}

	b := stravaganza.NewMessageBuilder()
	b.WithAttribute("from", "ortuman@jackal.im/yard")
	b.WithAttribute("to", "noelia@jackal.im/yard")
	b.WithChild(
		stravaganza.NewBuilder("body").
			WithText("I'll give thee a wind.").
			Build(),
	)
	msgID := uuid.New().String()
	b.WithAttribute("id", msgID)
	testMsg, _ := b.BuildMessage()

	elements := []streamqueue.Element{
		{Stanza: testMsg, H: 22},
	}

	hk := hook.NewHooks()
	sm := &Stream{
		cfg:         testSMConfig(),
		resMng:      resMngMock,
		stmQueueMap: streamqueue.NewQueueMap(),
		hk:          hk,
		logger:      kitlog.NewNopLogger(),
	}
	var streamErr *streamerror.Error
	oldStmMock := &c2sStreamMock{}
	oldStmMock.DisconnectFunc = func(sErr *streamerror.Error) <-chan error {
		streamErr = sErr
		errCh := make(chan error, 1)
		errCh <- nil
		return errCh
	}

	nc := testNonce()
	sq := streamqueue.New(
		oldStmMock, nc, elements, 10, 0, time.Second, time.Minute,
	)
	sm.stmQueueMap.Set(queueKey(jd), sq)

	sq.CancelTimers() // do not send R
	defer sq.CancelTimers()

	smID := encodeSMID(jd, nc)

	// when
	_ = sm.Start(context.Background())
	defer func() { _ = sm.Stop(context.Background()) }()

	halted, err := hk.Run(context.Background(), hook.C2SStreamElementReceived, &hook.ExecutionContext{
		Info: &hook.C2SStreamInfo{
			Element: stravaganza.NewBuilder("resume").
				WithAttribute(stravaganza.Namespace, streamNamespace).
				WithAttribute("previd", smID).
				WithAttribute("h", "21").
				Build(),
		},
		Sender: stmMock,
	})

	// then
	require.True(t, halted)
	require.Nil(t, err)

	require.True(t, resumed)

	require.Equal(t, streamerror.Conflict, streamErr.Reason)

	require.Len(t, sndElements, 2)

	require.Equal(t, "resumed", sndElements[0].Name())
	require.Equal(t, streamNamespace, sndElements[0].Attribute(stravaganza.Namespace))
	require.Equal(t, smID, sndElements[0].Attribute("previd"))
	require.Equal(t, "10", sndElements[0].Attribute("h"))

	require.Equal(t, msgID, sndElements[1].Attribute(stravaganza.ID))
}

func TestStream_ResumeRemote(t *testing.T) {
	// given
	jd, _ := jid.NewWithString("ortuman@jackal.im/yard", true)

	stmMock := &c2sStreamMock{}
	stmMock.IsAuthenticatedFunc = func() bool { return true }
	stmMock.IDFunc = func() stream.C2SID { return 1234 }
	stmMock.JIDFunc = func() *jid.JID { return jd }
	stmMock.UsernameFunc = func() string { return jd.Node() }
	stmMock.ResourceFunc = func() string { return jd.Resource() }
	stmMock.DisconnectFunc = func(_ *streamerror.Error) <-chan error { return nil }

	sndElements := make([]stravaganza.Element, 0)
	stmMock.SendElementFunc = func(elem stravaganza.Element) <-chan error {
		sndElements = append(sndElements, elem)
		return nil
	}
	var resumed bool
	stmMock.ResumeFunc = func(ctx context.Context, jd *jid.JID, pr *stravaganza.Presence, inf c2smodel.Info) error {
		resumed = true
		return nil
	}

	resMngMock := &resourceManagerMock{}
	resMngMock.GetResourceFunc = func(ctx context.Context, username string, resource string) (c2smodel.ResourceDesc, error) {
		return c2smodel.NewResourceDesc(
			"inst-1234",
			jd,
			xmpputil.MakePresence(jd, jd.ToBareJID(), stravaganza.AvailableType, nil),
			c2smodel.NewInfoMapFromMap(
				map[string]string{enabledInfoKey: "true"},
			),
		), nil
	}

	b := stravaganza.NewMessageBuilder()
	b.WithAttribute("from", "ortuman@jackal.im/yard")
	b.WithAttribute("to", "noelia@jackal.im/yard")
	b.WithChild(
		stravaganza.NewBuilder("body").
			WithText("I'll give thee a wind.").
			Build(),
	)
	msgID := uuid.New().String()
	b.WithAttribute("id", msgID)
	testMsg, _ := b.BuildMessage()

	elements := []streamqueue.Element{
		{Stanza: testMsg, H: 22},
	}

	nc := testNonce()

	clusterConnMngMock := &clusterConnManagerMock{}
	clusterConnMngMock.GetConnectionFunc = func(instanceID string) (clusterconnmanager.Conn, error) {
		clusterConnMock := &clusterConnMock{}
		clusterConnMock.StreamManagementFunc = func() clusterconnmanager.StreamManagement {
			stmMgmtServiceMock := &streamManagementServiceMock{}
			stmMgmtServiceMock.TransferQueueFunc = func(ctx context.Context, queueID string) (*clusterconnmanager.StreamQueue, error) {
				return &clusterconnmanager.StreamQueue{
					Elements: elements,
					Nonce:    nc,
					InH:      10,
					OutH:     0,
				}, nil
			}
			return stmMgmtServiceMock
		}
		return clusterConnMock, nil
	}

	hk := hook.NewHooks()
	sm := &Stream{
		cfg:            testSMConfig(),
		resMng:         resMngMock,
		stmQueueMap:    streamqueue.NewQueueMap(),
		clusterConnMng: clusterConnMngMock,
		hk:             hk,
		logger:         kitlog.NewNopLogger(),
	}

	smID := encodeSMID(jd, nc)

	// when
	_ = sm.Start(context.Background())
	defer func() { _ = sm.Stop(context.Background()) }()

	halted, err := hk.Run(context.Background(), hook.C2SStreamElementReceived, &hook.ExecutionContext{
		Info: &hook.C2SStreamInfo{
			Element: stravaganza.NewBuilder("resume").
				WithAttribute(stravaganza.Namespace, streamNamespace).
				WithAttribute("previd", smID).
				WithAttribute("h", "21").
				Build(),
		},
		Sender: stmMock,
	})

	// then
	require.True(t, halted)
	require.Nil(t, err)

	require.True(t, resumed)

	require.Len(t, sndElements, 2)

	require.Equal(t, "resumed", sndElements[0].Name())
	require.Equal(t, streamNamespace, sndElements[0].Attribute(stravaganza.Namespace))
	require.Equal(t, smID, sndElements[0].Attribute("previd"))
	require.Equal(t, "10", sndElements[0].Attribute("h"))

	require.Equal(t, msgID, sndElements[1].Attribute(stravaganza.ID))
}

func testSMConfig() Config {
	return Config{
		HibernateTime:      time.Minute,
		RequestAckInterval: time.Second,
		WaitForAckTimeout:  time.Second,
		MaxQueueSize:       10,
	}
}

func testNonce() []byte {
	nonce := make([]byte, nonceLength)
	for i := range nonce {
		nonce[i] = byte(rand.Intn(255) + 1)
	}
	return nonce
}
