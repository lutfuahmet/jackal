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

package xep0049

import (
	"context"
	"strings"

	kitlog "github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/jackal-xmpp/stravaganza"
	stanzaerror "github.com/jackal-xmpp/stravaganza/errors/stanza"
	"github.com/ortuman/jackal/pkg/hook"
	"github.com/ortuman/jackal/pkg/router"
	"github.com/ortuman/jackal/pkg/storage/repository"
	xmpputil "github.com/ortuman/jackal/pkg/util/xmpp"
)

const privateNamespace = "jabber:iq:private"

const (
	// ModuleName represents private module name.
	ModuleName = "private"

	// XEPNumber represents private XEP number.
	XEPNumber = "0049"
)

// Private represents a private (XEP-0049) module type.
type Private struct {
	router router.Router
	rep    repository.Private
	hk     *hook.Hooks
	logger kitlog.Logger
}

// New returns a new initialized Private instance.
func New(
	router router.Router,
	rep repository.Private,
	hk *hook.Hooks,
	logger kitlog.Logger,
) *Private {
	return &Private{
		rep:    rep,
		router: router,
		hk:     hk,
		logger: kitlog.With(logger, "module", ModuleName, "xep", XEPNumber),
	}
}

// Name returns private module name.
func (m *Private) Name() string { return ModuleName }

// StreamFeature returns private module stream feature.
func (m *Private) StreamFeature(_ context.Context, _ string) (stravaganza.Element, error) {
	return nil, nil
}

// ServerFeatures returns private server disco features.
func (m *Private) ServerFeatures(_ context.Context) ([]string, error) { return nil, nil }

// AccountFeatures returns private account disco features.
func (m *Private) AccountFeatures(_ context.Context) ([]string, error) { return nil, nil }

// MatchesNamespace tells whether namespace matches private module.
func (m *Private) MatchesNamespace(namespace string, serverTarget bool) bool {
	if serverTarget {
		return false
	}
	return namespace == privateNamespace
}

// ProcessIQ process a private iq.
func (m *Private) ProcessIQ(ctx context.Context, iq *stravaganza.IQ) error {
	fromJid := iq.FromJID()
	toJid := iq.ToJID()
	validTo := toJid.Node() == fromJid.Node()
	if !validTo {
		_, _ = m.router.Route(ctx, xmpputil.MakeErrorStanza(iq, stanzaerror.Forbidden))
		return nil
	}
	q := iq.ChildNamespace("query", privateNamespace)
	switch {
	case iq.IsGet() && q != nil:
		return m.getPrivate(ctx, iq, q)
	case iq.IsSet() && q != nil:
		return m.setPrivate(ctx, iq, q)
	default:
		_, _ = m.router.Route(ctx, xmpputil.MakeErrorStanza(iq, stanzaerror.BadRequest))
		return nil
	}
}

// Start starts private module.
func (m *Private) Start(_ context.Context) error {
	m.hk.AddHook(hook.UserDeleted, m.onUserDeleted, hook.DefaultPriority)

	level.Info(m.logger).Log("msg", "started private module")
	return nil
}

// Stop stops private module.
func (m *Private) Stop(_ context.Context) error {
	m.hk.RemoveHook(hook.UserDeleted, m.onUserDeleted)

	level.Info(m.logger).Log("msg", "stopped private module")
	return nil
}

func (m *Private) onUserDeleted(ctx context.Context, execCtx *hook.ExecutionContext) error {
	inf := execCtx.Info.(*hook.UserInfo)
	return m.rep.DeletePrivates(ctx, inf.Username)
}

func (m *Private) getPrivate(ctx context.Context, iq *stravaganza.IQ, q stravaganza.Element) error {
	if q.ChildrenCount() != 1 {
		_, _ = m.router.Route(ctx, xmpputil.MakeErrorStanza(iq, stanzaerror.NotAcceptable))
		return nil
	}
	prv := q.AllChildren()[0]
	ns := prv.Attribute(stravaganza.Namespace)

	isValidNS := isValidNamespace(ns)
	if prv.ChildrenCount() > 0 || !isValidNS {
		_, _ = m.router.Route(ctx, xmpputil.MakeErrorStanza(iq, stanzaerror.NotAcceptable))
		return nil
	}
	username := iq.FromJID().Node()

	prvElem, err := m.rep.FetchPrivate(ctx, ns, username)
	if err != nil {
		_, _ = m.router.Route(ctx, xmpputil.MakeErrorStanza(iq, stanzaerror.InternalServerError))
		return err
	}
	level.Info(m.logger).Log("msg", "fetched private XML", "username", username, "namespace", ns)

	qb := stravaganza.NewBuilder("query").
		WithAttribute(stravaganza.Namespace, privateNamespace)
	pb := stravaganza.NewBuilder(prv.Name()).
		WithAttribute(stravaganza.Namespace, ns)
	if prvElem != nil {
		pb.WithChildren(prvElem.AllChildren()...)
	}
	qb.WithChild(pb.Build())
	resIQ := xmpputil.MakeResultIQ(iq, qb.Build())

	_, _ = m.router.Route(ctx, resIQ)

	// run private fetched hook
	_, err = m.hk.Run(ctx, hook.PrivateFetched, &hook.ExecutionContext{
		Info: &hook.PrivateInfo{
			Username: username,
			Private:  prvElem,
		},
		Sender: m,
	})
	return err
}

func (m *Private) setPrivate(ctx context.Context, iq *stravaganza.IQ, q stravaganza.Element) error {
	if q.ChildrenCount() == 0 {
		_, _ = m.router.Route(ctx, xmpputil.MakeErrorStanza(iq, stanzaerror.NotAcceptable))
		return nil
	}
	username := iq.FromJID().Node()
	for _, prv := range q.AllChildren() {
		ns := prv.Attribute(stravaganza.Namespace)
		if !isValidNamespace(ns) {
			_, _ = m.router.Route(ctx, xmpputil.MakeErrorStanza(iq, stanzaerror.NotAcceptable))
			return nil
		}
		if err := m.rep.UpsertPrivate(ctx, prv, ns, username); err != nil {
			_, _ = m.router.Route(ctx, xmpputil.MakeErrorStanza(iq, stanzaerror.InternalServerError))
			return err
		}
		level.Info(m.logger).Log("msg", "saved private XML", "username", username, "namespace", ns)

		// run private updated hook
		_, err := m.hk.Run(ctx, hook.PrivateUpdated, &hook.ExecutionContext{
			Info: &hook.PrivateInfo{
				Username: username,
				Private:  prv,
			},
			Sender: m,
		})
		if err != nil {
			return err
		}
	}
	_, _ = m.router.Route(ctx, xmpputil.MakeResultIQ(iq, nil))
	return nil
}

func isValidNamespace(ns string) bool {
	return len(ns) > 0 && !strings.HasPrefix(ns, "jabber:") && !strings.HasPrefix(ns, "http://jabber.org/") && ns != "vcard-temp"
}
