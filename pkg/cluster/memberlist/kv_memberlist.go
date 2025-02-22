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

package memberlist

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"

	kitlog "github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/ortuman/jackal/pkg/cluster/instance"
	"github.com/ortuman/jackal/pkg/cluster/kv"
	kvtypes "github.com/ortuman/jackal/pkg/cluster/kv/types"
	"github.com/ortuman/jackal/pkg/hook"
	clustermodel "github.com/ortuman/jackal/pkg/model/cluster"
	"github.com/ortuman/jackal/pkg/version"
)

const (
	memberKeyPrefix   = "i://"
	memberValueFormat = "a=%s cv=%s"

	kvMemberListType = "kv"
)

// KVMemberList keeps and manages cluster memberlist set.
type KVMemberList struct {
	localPort int
	kv        kv.KV
	ctx       context.Context
	ctxCancel context.CancelFunc
	hk        *hook.Hooks
	logger    kitlog.Logger
	mu        sync.RWMutex
	members   map[string]clustermodel.Member
	stopCh    chan struct{}
}

// NewKVMemberList will create a new KVMemberList instance using the given configuration.
func NewKVMemberList(localPort int, kv kv.KV, hk *hook.Hooks, logger kitlog.Logger) *KVMemberList {
	ctx, cancelFn := context.WithCancel(context.Background())
	return &KVMemberList{
		localPort: localPort,
		kv:        kv,
		members:   make(map[string]clustermodel.Member),
		ctx:       ctx,
		ctxCancel: cancelFn,
		hk:        hk,
		logger:    logger,
		stopCh:    make(chan struct{}),
	}
}

// Start is used to join a cluster by registering instance member into the shared KV storage.
func (ml *KVMemberList) Start(ctx context.Context) error {
	if err := ml.join(ctx); err != nil {
		return err
	}
	level.Info(ml.logger).Log("msg", "registered local instance", "port", ml.localPort, "instance_id", instance.ID())

	// fetch current member list
	if err := ml.refreshMemberList(ctx); err != nil {
		return err
	}
	level.Info(ml.logger).Log("msg", "started memberlist", "type", kvMemberListType)

	return nil
}

// Stop unregisters instance member info from the cluster.
func (ml *KVMemberList) Stop(ctx context.Context) error {
	// stop watching changes...
	ml.ctxCancel()
	<-ml.stopCh

	// unregister local instance
	if err := ml.kv.Del(ctx, localMemberKey()); err != nil {
		return err
	}
	level.Info(ml.logger).Log("msg", "unregistered local instance", "port", ml.localPort)

	level.Info(ml.logger).Log("msg", "stopped memberlist", "type", kvMemberListType)
	return nil
}

// GetMember returns cluster member info associated to an identifier.
func (ml *KVMemberList) GetMember(instanceID string) (m clustermodel.Member, ok bool) {
	ml.mu.RLock()
	defer ml.mu.RUnlock()
	m, ok = ml.members[instanceID]
	return
}

// GetMembers returns all cluster registered members.
func (ml *KVMemberList) GetMembers() map[string]clustermodel.Member {
	ml.mu.RLock()
	defer ml.mu.RUnlock()
	res := make(map[string]clustermodel.Member)
	for k, v := range ml.members {
		res[k] = v
	}
	return res
}

func (ml *KVMemberList) join(ctx context.Context) error {
	lm, err := ml.getLocalMember()
	if err != nil {
		return err
	}
	kvVal := fmt.Sprintf(memberValueFormat, lm.String(), lm.APIVer)
	return ml.kv.Put(ctx, localMemberKey(), kvVal)
}

func (ml *KVMemberList) refreshMemberList(ctx context.Context) error {
	ch := make(chan error, 1)

	go func() {
		wCh := ml.kv.Watch(ml.ctx, memberKeyPrefix, false)

		ms, err := ml.getMembers(ctx)
		if err != nil {
			ch <- err
			return
		}
		ml.mu.Lock()
		for _, m := range ms {
			ml.members[m.InstanceID] = m
		}
		ml.mu.Unlock()

		// run updated member list hook
		err = ml.runHook(ctx, &hook.MemberListInfo{
			Registered: ms,
		})
		if err != nil {
			ch <- err
			return
		}
		close(ch) // signal update

		// watch changes
		for wResp := range wCh {
			if err := wResp.Err; err != nil {
				level.Warn(ml.logger).Log("msg", "error occurred watching memberlist", "err", err)
				continue
			}
			// process changes
			if err := ml.processKVEvents(ml.ctx, wResp.Events); err != nil {
				level.Warn(ml.logger).Log("msg", "failed to process memberlist changes", "err", err)
			}
		}
		close(ml.stopCh) // signal stop
	}()
	return <-ch
}

func (ml *KVMemberList) getMembers(ctx context.Context) ([]clustermodel.Member, error) {
	vs, err := ml.kv.GetPrefix(ctx, memberKeyPrefix)
	if err != nil {
		return nil, err
	}
	res := make([]clustermodel.Member, 0, len(vs))
	for k, val := range vs {
		if isLocalMemberKey(k) {
			continue // ignore local instance events
		}
		m, err := decodeClusterMember(k, string(val))
		if err != nil {
			level.Warn(ml.logger).Log("msg", "failed to decode cluster member", "err", err)
			continue
		}
		if m == nil {
			continue // discard local instance
		}
		res = append(res, *m)
	}
	return res, nil
}

func (ml *KVMemberList) getLocalMember() (*clustermodel.Member, error) {
	hostIP, err := getHostIP()
	if err != nil {
		return nil, err
	}
	return &clustermodel.Member{
		InstanceID: instance.ID(),
		Host:       hostIP,
		Port:       ml.localPort,
		APIVer:     version.ClusterAPIVersion,
	}, nil
}

func (ml *KVMemberList) processKVEvents(ctx context.Context, kvEvents []kvtypes.WatchEvent) error {
	var putMembers []clustermodel.Member
	var delMemberKeys []string

	ml.mu.Lock()
	for _, ev := range kvEvents {
		if isLocalMemberKey(ev.Key) {
			continue // ignore local instance events
		}
		switch ev.Type {
		case kvtypes.Put:
			m, err := decodeClusterMember(ev.Key, string(ev.Val))
			if err != nil {
				return err
			}
			ml.members[m.InstanceID] = *m
			putMembers = append(putMembers, *m)

			level.Info(ml.logger).Log("msg", "registered cluster member", "instance_id", m.InstanceID, "address", m.String(), "cluster_api_ver", m.APIVer.String())

		case kvtypes.Del:
			memberKey := strings.TrimPrefix(ev.Key, memberKeyPrefix)
			delete(ml.members, memberKey)
			delMemberKeys = append(delMemberKeys, memberKey)

			level.Info(ml.logger).Log("msg", "unregistered cluster member", "instance_id", memberKey)
		}
	}
	ml.mu.Unlock()

	// run updated hook
	return ml.runHook(ctx, &hook.MemberListInfo{
		Registered:       putMembers,
		UnregisteredKeys: delMemberKeys,
	})
}

func (ml *KVMemberList) runHook(ctx context.Context, inf *hook.MemberListInfo) error {
	_, err := ml.hk.Run(ctx, hook.MemberListUpdated, &hook.ExecutionContext{
		Info:   inf,
		Sender: ml,
	})
	return err
}

func decodeClusterMember(key, val string) (*clustermodel.Member, error) {
	instanceID := strings.TrimPrefix(key, memberKeyPrefix)

	var addr, minClusterVer string
	_, _ = fmt.Sscanf(val, memberValueFormat, &addr, &minClusterVer)

	var major, minor, patch uint
	_, _ = fmt.Sscanf(minClusterVer, "v%d.%d.%d", &major, &minor, &patch)

	host, sPort, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, _ := strconv.Atoi(sPort)
	return &clustermodel.Member{
		InstanceID: instanceID,
		Host:       host,
		Port:       port,
		APIVer:     version.NewVersion(major, minor, patch),
	}, nil
}

func localMemberKey() string {
	return memberKeyPrefix + instance.ID()
}

func isLocalMemberKey(k string) bool {
	return k == localMemberKey()
}

func getHostIP() (string, error) {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addresses {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String(), nil
			}
		}
	}
	return "", errors.New("instance: failed to get local ip")
}
