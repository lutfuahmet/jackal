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

package xep0313

import (
	"context"

	stanzaerror "github.com/jackal-xmpp/stravaganza/errors/stanza"
	"github.com/ortuman/jackal/pkg/router"

	"google.golang.org/protobuf/types/known/timestamppb"

	kitlog "github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/jackal-xmpp/stravaganza"
	"github.com/ortuman/jackal/pkg/hook"
	"github.com/ortuman/jackal/pkg/host"
	archivemodel "github.com/ortuman/jackal/pkg/model/archive"
	"github.com/ortuman/jackal/pkg/storage/repository"
	xmpputil "github.com/ortuman/jackal/pkg/util/xmpp"
)

const (
	mamNamespace         = "urn:xmpp:mam:2"
	extendedMamNamespace = "urn:xmpp:mam:2#extended"

	stanzaIDNamespace = "urn:xmpp:sid:0"
)

const (
	// ModuleName represents mam module name.
	ModuleName = "mam"

	// XEPNumber represents mam XEP number.
	XEPNumber = "0313"
)

// Config contains mam module configuration options.
type Config struct {
	// QueueSize defines maximum number of archive messages stanzas.
	// When the limit is reached, the oldest message will be purged to make room for the new one.
	QueueSize int `fig:"queue_size" default:"1000"`
}

// Mam represents a mam (XEP-0313) module type.
type Mam struct {
	cfg    Config
	hosts  *host.Hosts
	router router.Router
	hk     *hook.Hooks
	rep    repository.Repository
	logger kitlog.Logger
}

// New returns a new initialized mam instance.
func New(
	cfg Config,
	router router.Router,
	hosts *host.Hosts,
	rep repository.Repository,
	hk *hook.Hooks,
	logger kitlog.Logger,
) *Mam {
	return &Mam{
		cfg:    cfg,
		router: router,
		hosts:  hosts,
		rep:    rep,
		hk:     hk,
		logger: kitlog.With(logger, "module", ModuleName, "xep", XEPNumber),
	}
}

// Name returns mam module name.
func (m *Mam) Name() string { return ModuleName }

// StreamFeature returns mam module stream feature.
func (m *Mam) StreamFeature(_ context.Context, _ string) (stravaganza.Element, error) {
	return nil, nil
}

// ServerFeatures returns mam server disco features.
func (m *Mam) ServerFeatures(_ context.Context) ([]string, error) {
	return nil, nil
}

// AccountFeatures returns mam account disco features.
func (m *Mam) AccountFeatures(_ context.Context) ([]string, error) {
	return []string{mamNamespace, extendedMamNamespace}, nil
}

// Start starts stream module.
func (m *Mam) Start(_ context.Context) error {
	m.hk.AddHook(hook.C2SStreamElementReceived, m.onMessageRecv, hook.HighestPriority)
	m.hk.AddHook(hook.S2SInStreamElementReceived, m.onMessageRecv, hook.HighestPriority)

	m.hk.AddHook(hook.C2SStreamMessageRouted, m.onMessageRouted, hook.LowestPriority+1)
	m.hk.AddHook(hook.S2SInStreamMessageRouted, m.onMessageRouted, hook.LowestPriority+1)

	m.hk.AddHook(hook.UserDeleted, m.onUserDeleted, hook.DefaultPriority)

	level.Info(m.logger).Log("msg", "started mam module")
	return nil
}

// Stop stops stream module.
func (m *Mam) Stop(_ context.Context) error {
	m.hk.RemoveHook(hook.C2SStreamElementReceived, m.onMessageRecv)
	m.hk.RemoveHook(hook.S2SInStreamElementReceived, m.onMessageRecv)
	m.hk.RemoveHook(hook.C2SStreamMessageRouted, m.onMessageRouted)
	m.hk.RemoveHook(hook.S2SInStreamMessageRouted, m.onMessageRouted)
	m.hk.RemoveHook(hook.UserDeleted, m.onUserDeleted)

	level.Info(m.logger).Log("msg", "stopped mam module")
	return nil
}

// MatchesNamespace tells whether namespace matches blocklist module.
func (m *Mam) MatchesNamespace(namespace string, serverTarget bool) bool {
	if serverTarget {
		return false
	}
	return namespace == mamNamespace
}

// ProcessIQ process a mam iq.
func (m *Mam) ProcessIQ(ctx context.Context, iq *stravaganza.IQ) error {
	fromJID := iq.FromJID()
	toJID := iq.ToJID()
	if fromJID.Node() != toJID.Node() {
		_, _ = m.router.Route(ctx, xmpputil.MakeErrorStanza(iq, stanzaerror.Forbidden))
		return nil
	}
	switch {
	case iq.IsGet() && iq.ChildNamespace("metadata", mamNamespace) != nil:
		return m.sendArchiveMetadata(ctx, iq)
	}
	return nil
}

func (m *Mam) sendArchiveMetadata(ctx context.Context, iq *stravaganza.IQ) error {
	metadata, err := m.rep.FetchArchiveMetadata(ctx, iq.FromJID().Node())
	if err != nil {
		_, _ = m.router.Route(ctx, xmpputil.MakeErrorStanza(iq, stanzaerror.InternalServerError))
		return err
	}
	// send reply
	metadataBuilder := stravaganza.NewBuilder("metadata").WithAttribute(stravaganza.Namespace, mamNamespace)

	startBuilder := stravaganza.NewBuilder("start")
	if metadata != nil {
		startBuilder.WithAttribute("id", metadata.StartId)
		startBuilder.WithAttribute("timestamp", metadata.StartTimestamp)
	}
	endBuilder := stravaganza.NewBuilder("end")
	if metadata != nil {
		endBuilder.WithAttribute("id", metadata.EndId)
		endBuilder.WithAttribute("timestamp", metadata.EndTimestamp)
	}

	metadataBuilder.WithChildren(startBuilder.Build(), endBuilder.Build())

	resIQ := xmpputil.MakeResultIQ(iq, metadataBuilder.Build())
	_, _ = m.router.Route(ctx, resIQ)

	return nil
}

func (m *Mam) onMessageRecv(_ context.Context, execCtx *hook.ExecutionContext) error {
	switch inf := execCtx.Info.(type) {
	case *hook.C2SStreamInfo:
		inf.Element = m.addMessageStanzaID(inf.Element)
	case *hook.S2SStreamInfo:
		inf.Element = m.addMessageStanzaID(inf.Element)
	}
	return nil
}

func (m *Mam) onMessageRouted(ctx context.Context, execCtx *hook.ExecutionContext) error {
	var elem stravaganza.Element

	switch inf := execCtx.Info.(type) {
	case *hook.C2SStreamInfo:
		elem = inf.Element
	case *hook.S2SStreamInfo:
		elem = inf.Element
	}
	return m.handleRoutedElement(ctx, elem)
}

func (m *Mam) onUserDeleted(ctx context.Context, execCtx *hook.ExecutionContext) error {
	inf := execCtx.Info.(*hook.UserInfo)
	return m.rep.DeleteArchive(ctx, inf.Username)
}

func (m *Mam) addMessageStanzaID(elem stravaganza.Element) stravaganza.Element {
	msg, ok := elem.(*stravaganza.Message)
	if !ok {
		return elem
	}
	if !isMessageArchievable(msg) {
		return elem
	}

	if !m.hosts.IsLocalHost(msg.ToJID().Domain()) {
		return elem
	}
	return xmpputil.MakeStanzaIDMessage(msg)
}

func (m *Mam) handleRoutedElement(ctx context.Context, elem stravaganza.Element) error {
	msg, ok := elem.(*stravaganza.Message)
	if !ok {
		return nil
	}
	if !isMessageArchievable(msg) {
		return nil
	}

	fromJID := msg.FromJID()
	if m.hosts.IsLocalHost(fromJID.Domain()) {
		if err := m.archiveMessage(ctx, msg, fromJID.Node(), uuid.New().String()); err != nil {
			return err
		}
	}
	toJID := msg.ToJID()
	if !m.hosts.IsLocalHost(toJID.Domain()) {
		return nil
	}
	stanzaID := msg.ChildNamespace("stanza-id", stanzaIDNamespace)
	if stanzaID == nil {
		return nil
	}
	return m.archiveMessage(ctx, msg, toJID.Node(), stanzaID.Attribute("id"))
}

func (m *Mam) archiveMessage(ctx context.Context, message *stravaganza.Message, archiveID, id string) error {
	return m.rep.InTransaction(ctx, func(ctx context.Context, tx repository.Transaction) error {
		err := tx.InsertArchiveMessage(ctx, &archivemodel.Message{
			ArchiveId: archiveID,
			Id:        id,
			FromJid:   message.FromJID().String(),
			ToJid:     message.ToJID().String(),
			Message:   message.Proto(),
			Stamp:     timestamppb.Now(),
		})
		if err != nil {
			return err
		}
		return tx.DeleteArchiveOldestMessages(ctx, archiveID, m.cfg.QueueSize)
	})
}

func isMessageArchievable(msg *stravaganza.Message) bool {
	return (msg.IsNormal() || msg.IsChat()) && msg.IsMessageWithBody()
}
