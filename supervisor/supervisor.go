package supervisor

import (
	"errors"

	"github.com/CIRCL/pbtc/adaptor"
	"github.com/CIRCL/pbtc/logger"
	"github.com/CIRCL/pbtc/manager"
	"github.com/CIRCL/pbtc/repository"
	"github.com/CIRCL/pbtc/server"
	"github.com/CIRCL/pbtc/tracker"
)

type Supervisor struct {
	logr map[string]adaptor.Logger
	repo map[string]adaptor.Repository
	tkr  map[string]adaptor.Tracker
	svr  map[string]adaptor.Server
	pro  map[string]adaptor.Processor
	mgr  map[string]adaptor.Manager
	log  adaptor.Log
}

func New() (*Supervisor, error) {
	// load configuration file
	cfg := &Config{}
	err := gcfg.ReadFileInto(cfg, "pbtc.cfg")
	if err != nil {
		return nil, err
	}

	// initialize struct with maps
	supervisor := &Supervisor{
		logr: make(map[string]adaptor.Logger),
		repo: make(map[string]adaptor.Repository),
		tkr:  make(map[string]adaptor.Tracker),
		svr:  make(map[string]adaptor.Server),
		pro:  make(map[string]adaptor.Processor),
		mgr:  make(map[string]adaptor.Manager),
	}

	// initialize loggers so we can start logging
	for name, logr_cfg := range cfg.Logger {
		supervisor.logr[name] = initLogger(logr_cfg)
	}

	if len(supervisor.logr) == 0 {
		logr, err := logger.New()
		if err != nil {
			return nil, err
		}

		supervisor.logr[""] = logr
		supervisor.log = supervisor.logr[""].GetLog("sup")
		supervisor.log.Warning("No logger module defined")
	} else {
		_, ok := supervisor.logr[""]
		if !ok {
			for _, v := range supervisor.logr {
				supervisor.logr[""] = v
				supervisor.log = supervisor.logr[""].GetLog("sup")
				supervisor.log.Notice("No default logger defined")
				break
			}
		} else {
			supervisor.log = supervisor.logr[""].GetLog("sup")
		}
	}

	// initialize remaining modules
	for name, repo_cfg := range cfg.Repository {
		supervisor.repo[name] = initRepository(repo_cfg)
	}

	for name, tkr_cfg := range cfg.Tracker {
		supervisor.tkr[name] = initTracker(tkr_cfg)
	}

	for name, svr_cfg := range cfg.Server {
		supervisor.svr[name] = initServer(svr_cfg)
	}

	for name, pro_cfg := range cfg.Processor {
		supervisor.pro[name] = initProcessor(pro_cfg)
	}

	for name, mgr_cfg := range cfg.Manager {
		supervisor.mgr[name] = initManager(mgr_cfg)
	}

	// check remaining modules for missing values
	if len(supervisor.repo) == 0 {
		supervisor.log.Warning("No repository module defined")
		repo, err := repository.New()
		if err != nil {
			return nil, err
		}

		supervisor.repo[""] = repo
	}

	if len(supervisor.tkr) == 0 {
		supervisor.log.Warning("No tracker module defined")
		tkr, err := tracker.New()
		if err != nil {
			return nil, err
		}

		supervisor.tkr[""] = tkr
	}

	if len(supervisor.svr) == 0 {
		supervisor.log.Notice("No server module defined")
		svr, err := server.New()
		if err != nil {
			return nil, err
		}

		supervisor.svr[""] = svr
	}

	if len(supervisor.pro) == 0 {
		supervisor.log.Notice("No processor module defined")
	}

	if len(supervisor.mgr) == 0 {
		supervisor.log.Warning("No manager module defined")
		mgr, err := manager.New()
		if err != nil {
			return nil, err
		}

		supervisor.mgr[""] = mgr
	}

	// check remaining modules for missing default module
	_, ok := supervisor.repo[""]
	if !ok {
		for _, v := range supervisor.repo {
			supervisor.log.Notice("No default repository defined")
			supervisor.repo[""] = v
			break
		}
	}

	_, ok = supervisor.tkr[""]
	if !ok {
		for _, v := range supervisor.tkr {
			supervisor.log.Notice("No default tracker defined")
			supervisor.tkr[""] = v
			break
		}
	}

	_, ok = supervisor.svr[""]
	if !ok {
		for _, v := range supervisor.svr {
			supervisor.log.Notice("No default server defined")
			supervisor.svr[""] = v
			break
		}
	}

	_, ok = supervisor.mgr[""]
	if !ok {
		for _, v := range supervisor.mgr {
			supervisor.log.Notice("No default manager defined")
			supervisor.mgr[""] = v
			break
		}
	}

	if cfg.Processor[""] == nil {
		for k, v := range cfg.Processor {
			cfg.Processor[""] = v
			delete(cfg.Processor, k)
			break
		}
	}

	// initialize supervisor struct
	sup := &Supervisor{
		log:  logr.GetLog("sup"),
		repo: make(map[string]adaptor.Repository),
		trk:  make(map[string]adaptor.Tracker),
		svr:  make(map[string]adaptor.Server),
		mgr:  make(map[string]adaptor.Manager),
		pro:  make(map[string]adaptor.Processor),
	}

	// initialize repositories
	for k, v := range cfg.Repository {
		repo, err := repository.New(
			repository.SetLog(logr.GetLog("repo"+k)),
			repository.SetSeedsList(v.Seeds_list...),
			repository.SetSeedsPort(v.Seeds_port),
			repository.SetBackupPath(v.Backup_path),
			repository.SetBackupRate(v.Backup_rate),
			repository.SetNodeLimit(v.Node_limit),
		)
		if err != nil {
			return nil, err
		}

		logr.SetLevel("repo"+k, v.Log_level)
		sup.repo[k] = repo
	}

	// initialize trackers
	for k, v := range cfg.Tracker {
		tkr, err := tracker.New(
			tracker.SetLog(logr.GetLog("tkr" + k)),
		)
		if err != nil {
			return nil, err
		}

		logr.SetLevel("tkr"+k, tkr.Log_level)
	}

	// initialize servers

	// initialize managers

	// initialize processors

	return sup, nil
}

func initLogger(lgr_cfg *LoggerConfig) adaptor.Logger {
	return nil
}

func initRepository(repo_cfg *RepositoryConfig) adaptor.Repository {
	return nil
}

func initTracker(tkr_cfg *TrackerConfig) adaptor.Tracker {
	return nil
}

func initServer(svr_cfg *ServerConfig) adaptor.Server {
	return nil
}

func initProcessor(pro_cfg *ProcessorConfig) adaptor.Processor {
	return nil
}

func initManager(mgr_cfg *ManagerConfig) adaptor.Manager {
	return nil
}

func (supervisor *Supervisor) Start() {
}

func (supervisor *Supervisor) Stop() {
}
