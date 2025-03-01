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

package jackal

import (
	"context"
	"flag"
	"fmt"
	"github.com/ortuman/jackal/pkg/util/rlimit"
	"io"
	"math/rand"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	streamqueue "github.com/ortuman/jackal/pkg/module/xep0198/queue"

	kitlog "github.com/go-kit/log"
	"github.com/go-kit/log/level"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	adminserver "github.com/ortuman/jackal/pkg/admin/server"
	"github.com/ortuman/jackal/pkg/auth/pepper"
	"github.com/ortuman/jackal/pkg/c2s"
	clusterconnmanager "github.com/ortuman/jackal/pkg/cluster/connmanager"
	"github.com/ortuman/jackal/pkg/cluster/kv"
	"github.com/ortuman/jackal/pkg/cluster/memberlist"
	"github.com/ortuman/jackal/pkg/cluster/resourcemanager"
	clusterrouter "github.com/ortuman/jackal/pkg/cluster/router"
	clusterserver "github.com/ortuman/jackal/pkg/cluster/server"
	"github.com/ortuman/jackal/pkg/component"
	"github.com/ortuman/jackal/pkg/component/extcomponentmanager"
	"github.com/ortuman/jackal/pkg/component/xep0114"
	"github.com/ortuman/jackal/pkg/hook"
	"github.com/ortuman/jackal/pkg/host"
	"github.com/ortuman/jackal/pkg/log"
	"github.com/ortuman/jackal/pkg/module"
	"github.com/ortuman/jackal/pkg/router"
	"github.com/ortuman/jackal/pkg/s2s"
	"github.com/ortuman/jackal/pkg/shaper"
	"github.com/ortuman/jackal/pkg/storage"
	"github.com/ortuman/jackal/pkg/storage/repository"
	"github.com/ortuman/jackal/pkg/util/crashreporter"
	"github.com/ortuman/jackal/pkg/version"
)

const (
	defaultBootstrapTimeout = time.Minute
	defaultShutdownTimeout  = time.Second * 30

	envConfigFile = "JACKAL_CONFIG_FILE"
)

var logoStr = []string{
	`     __               __            __   `,
	`    |__|____    ____ |  | _______  |  |  `,
	`    |  \__  \ _/ ___\|  |/ /\__  \ |  |  `,
	`    |  |/ __ \\  \___|    <  / __ \|  |__`,
	`/\__|  (____  /\___  >__|_ \(____  /____/`,
	`\______|    \/     \/     \/     \/      `,
}

const usageStr = `
Usage: jackal [options]
Server Options:
    --config <file>    Configuration file path
Common Options:
    --help             Show this message
`

type starter interface {
	Start(ctx context.Context) error
}

type stopper interface {
	Stop(ctx context.Context) error
}

type startStopper interface {
	starter
	stopper
}

// Jackal is the root data structure for Jackal.
type Jackal struct {
	output io.Writer
	args   []string

	peppers *pepper.Keys
	hk      *hook.Hooks

	kv         kv.KV
	memberList memberlist.MemberList
	resMng     resourcemanager.Manager

	rep repository.Repository

	shapers        shaper.Shapers
	hosts          *host.Hosts
	clusterConnMng *clusterconnmanager.Manager

	localRouter    *c2s.LocalRouter
	clusterRouter  *clusterrouter.Router
	s2sOutProvider *s2s.OutProvider
	router         router.Router
	mods           *module.Modules
	comps          *component.Components
	stmQueueMap    *streamqueue.QueueMap
	extCompMng     *extcomponentmanager.Manager

	starters []starter
	stoppers []stopper

	waitStopCh chan os.Signal

	logger kitlog.Logger
}

// New makes a new Jackal.
func New(output io.Writer, args []string) *Jackal {
	return &Jackal{
		output:     output,
		args:       args,
		waitStopCh: make(chan os.Signal, 1),
		kv:         kv.NewNop(),
		memberList: memberlist.NewNop(),
	}
}

// Run starts Jackal running, and blocks until a Jackal stops.
func (j *Jackal) Run() error {
	// seed the math/rand RNG from crypto/rand.
	rand.Seed(time.Now().UnixNano())

	defer crashreporter.RecoverAndReportPanic()

	fs := flag.NewFlagSet("jackal", flag.ExitOnError)
	fs.SetOutput(j.output)

	var configFile string
	var showVersion, showUsage bool

	fs.BoolVar(&showUsage, "help", false, "Show this message")
	fs.BoolVar(&showVersion, "version", false, "Print version information.")
	fs.StringVar(&configFile, "config", "config.yaml", "Configuration file path.")

	fs.Usage = func() {
		for i := range logoStr {
			_, _ = fmt.Fprintf(j.output, "%s\n", logoStr[i])
		}
		_, _ = fmt.Fprintf(j.output, "%s\n", usageStr)
	}
	_ = fs.Parse(j.args[1:])

	// print usage
	if showUsage {
		fs.Usage()
		return nil
	}
	// print version
	if showVersion {
		_, _ = fmt.Fprintf(j.output, "jackal version: %v\n", version.Version)
		return nil
	}
	// if present, override config file url with env var
	if envCfgFile := os.Getenv(envConfigFile); len(envCfgFile) > 0 {
		configFile = envCfgFile
	}
	// load configuration
	cfg, err := loadConfig(configFile)
	if err != nil {
		return err
	}
	// init logger
	j.logger = log.NewDefaultLogger(cfg.Logger.Level, cfg.Logger.Format)

	level.Info(j.logger).Log("msg", "jackal is starting...",
		"version", version.Version,
		"go_ver", runtime.Version(),
		"go_os", runtime.GOOS,
		"go_arch", runtime.GOARCH,
	)
	// Allocate a block of memory to alter GC behaviour. See https://github.com/golang/go/issues/23044
	ballast := make([]byte, cfg.MemoryBallastSize)
	runtime.KeepAlive(ballast)

	// enable gRPC prometheus histograms
	grpc_prometheus.EnableHandlingTimeHistogram()

	// set maximum opened files limit
	if err := rlimit.SetRLimit(); err != nil {
		return err
	}

	// init pepper keys
	peppers, err := pepper.NewKeys(cfg.Peppers)
	if err != nil {
		return err
	}
	j.peppers = peppers

	// init hooks
	j.hk = hook.NewHooks()

	// init cluster
	if err := j.initCluster(cfg.Cluster); err != nil {
		return err
	}

	// init repository
	if err := j.initRepository(cfg.Storage); err != nil {
		return err
	}
	// init C2S/S2S routers
	if err := j.initHosts(cfg.Hosts); err != nil {
		return err
	}
	if err := j.initShapers(cfg.Shapers); err != nil {
		return err
	}
	j.initS2SOut(cfg.S2S.Out)
	j.initRouters()

	// init components & modules
	j.initComponents()

	if err := j.initModules(cfg.Modules); err != nil {
		return err
	}

	// init admin server
	j.initAdminServer(cfg.Admin)

	// init cluster server
	if cfg.Cluster.IsEnabled() {
		j.initClusterServer(cfg.Cluster.Server)
	}

	// init C2S/S2S listeners
	if err := j.initListeners(cfg.C2S.Listeners, cfg.S2S.Listeners, cfg.Components.Listeners, cfg.Components.Secret); err != nil {
		return err
	}
	// init HTTP server
	j.registerStartStopper(newHTTPServer(cfg.HTTP.Port, j.logger))

	if err := j.bootstrap(); err != nil {
		return err
	}
	// ...wait for stop signal to shut down
	sig := j.waitForStopSignal()
	level.Info(j.logger).Log("msg", "received stop signal... shutting down...",
		"signal", sig.String(),
	)
	return j.shutdown()
}

func (j *Jackal) initCluster(cfg ClusterConfig) error {
	switch cfg.Type {
	case kvClusterType:
		if err := j.initKVStore(cfg.KV); err != nil {
			return err
		}
		fallthrough

	case noneClusterType:
		j.memberList = memberlist.NewKVMemberList(cfg.Server.Port, j.kv, j.hk, j.logger)
		j.resMng = resourcemanager.NewKVManager(j.kv, j.hk, j.logger)

	default:
		return fmt.Errorf("unrecognized cluster type: %s", cfg.Type)
	}
	// init cluster connection manager
	j.clusterConnMng = clusterconnmanager.NewManager(j.hk, j.logger)

	j.registerStartStopper(j.clusterConnMng)
	j.registerStartStopper(j.resMng)
	j.registerStartStopper(j.memberList)
	return nil
}

func (j *Jackal) initKVStore(cfg kv.Config) error {
	kvs, err := kv.New(cfg, j.logger)
	if err != nil {
		return err
	}
	j.kv = kv.NewMeasured(kvs)
	j.registerStartStopper(j.kv)
	return nil
}

func (j *Jackal) initRepository(cfg storage.Config) error {
	rep, err := storage.New(cfg, j.logger)
	if err != nil {
		return err
	}
	j.rep = rep
	j.registerStartStopper(j.rep)
	return nil
}

func (j *Jackal) initHosts(configs host.Configs) error {
	h, err := host.NewHosts(configs)
	if err != nil {
		return err
	}
	j.hosts = h
	return nil
}

func (j *Jackal) initShapers(configs []shaper.Config) error {
	j.shapers = make(shaper.Shapers, 0)
	for _, cfg := range configs {
		shp, err := shaper.New(cfg)
		if err != nil {
			return err
		}
		j.shapers = append(j.shapers, shp)

		level.Info(j.logger).Log("msg", "registered shaper configuration",
			"name", cfg.Name,
			"max_sessions", cfg.MaxSessions,
			"limit", cfg.Rate.Limit,
			"burst", cfg.Rate.Burst,
		)
	}
	return nil
}

func (j *Jackal) initListeners(
	c2sListenersCfg c2s.ListenersConfig,
	s2sListenersCfg s2s.ListenersConfig,
	cmpListenersCfg xep0114.ListenersConfig,
	cmpSecretKey string,
) error {
	// c2s listeners
	c2sListeners := c2s.NewListeners(
		c2sListenersCfg,
		j.hosts,
		j.router,
		j.comps,
		j.mods,
		j.resMng,
		j.rep,
		j.peppers,
		j.shapers,
		j.hk,
		j.logger,
	)
	for _, ln := range c2sListeners {
		j.registerStartStopper(ln)
	}

	// s2s listeners
	if len(s2sListenersCfg) > 0 {
		s2sInHub := s2s.NewInHub(j.logger)
		j.registerStartStopper(s2sInHub)

		s2sListeners := s2s.NewListeners(
			s2sListenersCfg,
			j.hosts,
			j.router,
			j.comps,
			j.mods,
			j.s2sOutProvider,
			s2sInHub,
			j.kv,
			j.shapers,
			j.hk,
			j.logger,
		)
		for _, ln := range s2sListeners {
			j.registerStartStopper(ln)
		}
	}

	// external component listeners
	cmpListeners := xep0114.NewListeners(
		cmpListenersCfg,
		cmpSecretKey,
		j.hosts,
		j.comps,
		j.extCompMng,
		j.router,
		j.shapers,
		j.hk,
		j.logger,
	)
	for _, ln := range cmpListeners {
		j.registerStartStopper(ln)
	}
	return nil
}

func (j *Jackal) initS2SOut(cfg s2s.OutConfig) {
	j.s2sOutProvider = s2s.NewOutProvider(cfg, j.hosts, j.kv, j.shapers, j.hk, j.logger)
	j.registerStartStopper(j.s2sOutProvider)
}

func (j *Jackal) initRouters() {
	// init C2S router
	j.localRouter = c2s.NewLocalRouter(j.hosts)
	j.clusterRouter = clusterrouter.New(j.clusterConnMng)

	c2sRouter := c2s.NewRouter(j.localRouter, j.clusterRouter, j.resMng, j.rep, j.hk, j.logger)
	s2sRouter := s2s.NewRouter(j.s2sOutProvider)

	// init global router
	j.router = router.New(j.hosts, c2sRouter, s2sRouter)
	j.registerStartStopper(j.router)
	return
}

func (j *Jackal) initComponents() {
	j.comps = component.NewComponents(nil, j.hk, j.logger)
	j.extCompMng = extcomponentmanager.New(j.kv, j.clusterConnMng, j.comps, j.logger)

	j.registerStartStopper(j.comps)
	j.registerStartStopper(j.extCompMng)
}

func (j *Jackal) initModules(cfg ModulesConfig) error {
	var mods []module.Module

	// enabled modules
	enabled := cfg.Enabled
	if len(enabled) == 0 {
		enabled = defaultModules
	}
	for _, mName := range enabled {
		fn, ok := modFns[mName]
		if !ok {
			return fmt.Errorf("main: unrecognized module name: %s", mName)
		}
		mods = append(mods, fn(j, &cfg))
	}
	j.mods = module.NewModules(mods, j.hosts, j.router, j.hk, j.logger)
	j.registerStartStopper(j.mods)
	return nil
}

func (j *Jackal) initAdminServer(cfg adminserver.Config) {
	adminSrv := adminserver.New(cfg, j.rep, j.peppers, j.hk, j.logger)
	j.registerStartStopper(adminSrv)
}

func (j *Jackal) initClusterServer(cfg clusterserver.Config) {
	clusterSrv := clusterserver.New(cfg, j.localRouter, j.comps, j.stmQueueMap, j.logger)
	j.registerStartStopper(clusterSrv)
	return
}

func (j *Jackal) registerStartStopper(ss startStopper) {
	if ss == nil {
		return
	}
	j.starters = append(j.starters, ss)
	j.stoppers = append([]stopper{ss}, j.stoppers...)
}

func (j *Jackal) bootstrap() error {
	// spin up all service subsystems
	ctx, cancel := context.WithTimeout(context.Background(), defaultBootstrapTimeout)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		// invoke all registered starters...
		for _, s := range j.starters {
			if err := s.Start(ctx); err != nil {
				errCh <- err
				return
			}
		}
		errCh <- nil
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (j *Jackal) shutdown() error {
	// wait until shutdown has been completed
	ctx, cancel := context.WithTimeout(context.Background(), defaultShutdownTimeout)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		// invoke all registered stoppers...
		for _, st := range j.stoppers {
			if err := st.Stop(ctx); err != nil {
				errCh <- err
				return
			}
		}
		errCh <- nil
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (j *Jackal) waitForStopSignal() os.Signal {
	signal.Notify(j.waitStopCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	return <-j.waitStopCh
}
