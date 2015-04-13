package all

import (
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/wire"
	"github.com/op/go-logging"
)

// Manager is the module responsible for managing the connections to peers and
// keep them in line with application level state and requirements. It accepts
// inbound connections, establishes the desired number of outgoing connections
// and manages the creation and disposal of peers. It will use a provided
// repository to get addresses to connect to and notifies it about changes
// relevant to address selection.
type Manager struct {
	repo        *Repository
	peerIndex   map[string]*peer
	listenIndex map[string]*net.TCPListener
	sigPeer     chan struct{}
	sigConn     chan struct{}
	peerNew     chan *peer
	peerDone    chan *peer
	connTicker  *time.Ticker
	wg          *sync.WaitGroup
	state       uint32
	network     wire.BitcoinNet
	version     uint32
	nonce       uint64
}

// NewManager returns a new manager with all necessary variables initialized.
func NewManager() *Manager {
	mgr := &Manager{
		peerIndex:   make(map[string]*peer),
		listenIndex: make(map[string]*net.TCPListener),
		sigPeer:     make(chan struct{}, 1),
		sigConn:     make(chan struct{}, 1),
		peerNew:     make(chan *peer, bufferManagerNew),
		peerDone:    make(chan *peer, bufferManagerDone),
		connTicker:  time.NewTicker(time.Second / maxConnsPerSec),
		wg:          &sync.WaitGroup{},
		state:       stateIdle,
	}

	return mgr
}

// Start starts the manager, with run-time options passed in as parameters. This allows
// us to stop and restart the manager with a different protocol version, network or even
// repository of nodes.
func (mgr *Manager) Start(repo *Repository, network wire.BitcoinNet, version uint32) {
	// we can only start the manager if it is in idle state and ready to be started
	if !atomic.CompareAndSwapUint32(&mgr.state, stateIdle, stateBusy) {
		return
	}

	log := logging.MustGetLogger("pbtc")
	log.Info("Manager starting")

	// set the parameters for the nodes and connections we will create
	mgr.repo = repo
	mgr.network = network
	mgr.version = version

	// listen on local IPs for incoming peers
	log.Debug("Creating listeners & listen handlers")
	mgr.createListeners()

	// here, we start all handlers that execute concurrently
	// we add them to the waitgrop so that we can cleanly shutdown later
	log.Debug("Launching other handlers")
	mgr.wg.Add(2)
	go mgr.handleConnections()
	go mgr.handlePeers()

	// at this point, start-up is complete and we can set the new state
	log.Info("Manager started")
	atomic.StoreUint32(&mgr.state, stateRunning)
}

// Stop cleanly shuts down the manager so it can be restarted later.
func (mgr *Manager) Stop() {
	// we can only stop the manager if we are currently in running state
	if !atomic.CompareAndSwapUint32(&mgr.state, stateRunning, stateBusy) {
		return
	}

	log := logging.MustGetLogger("pbtc")
	log.Info("Manager stopping")

	// first we will stop every peer - this is a blocking operation
	log.Debug("Stopping peers")
	for _, peer := range mgr.peerIndex {
		peer.Stop()
	}

	// here, we close the channel to signal the connection handler to stop
	log.Debug("Signaling connection handler")
	close(mgr.sigConn)

	// the listener handler already quits after launching all listeners
	// we thus only need to close all listeners and wait for their routines to stop
	log.Debug("Closing listeners")
	for _, listener := range mgr.listenIndex {
		listener.Close()
	}

	// finally, we signal the peer listener to stop processing as well
	log.Debug("Signaling peer handler")
	close(mgr.sigPeer)

	// we then wait for all handlers to finish cleanly
	log.Debug("Waiting for handlers to return")
	mgr.wg.Wait()

	// at this point, all handlers have stopped and we are back in idle state
	log.Info("Manager stopped")
	atomic.StoreUint32(&mgr.state, stateIdle)
}

// createListeners tries to start a listener on every local IP to accept
// connections. It should be called as a go routine.
func (mgr *Manager) createListeners() {
	log := logging.MustGetLogger("pbtc")
	log.Debug("Creating listeners")

	// get all IPs on local interfaces and iterate through them
	ips := FindLocalIPs()
	log.Debug("%v local IPs found", len(ips))

	for _, ip := range ips {
		// if we can't convert into a TCP address, skip
		addr, err := net.ResolveTCPAddr("tcp", ip.String()+":"+strconv.Itoa(GetDefaultPort()))
		if err != nil {
			log.Warning("Could not convert %s to TCP address", ip)
			continue
		}

		// if we are already listening on this address, skip
		_, ok := mgr.listenIndex[addr.String()]
		if ok {
			log.Notice("Already listening on address")
			continue
		}

		// if we can't create the listener, skip
		listener, err := net.ListenTCP("tcp", addr)
		if err != nil {
			log.Warning("Could not create TCP listener")
			continue
		}

		// add the listener to our index and start an accepting handler
		// we again need to add it to the waitgroup if we want to exit cleanly
		log.Debug("Adding & launching listener: %v", listener.Addr())
		mgr.listenIndex[addr.String()] = listener
		mgr.wg.Add(1)
		go mgr.handleListener(listener)
	}
}

// handleConnections attempts to establish new connections at the configured
// rate as long as we are not at the maximum number of connections.
func (mgr *Manager) handleConnections() {
	log := logging.MustGetLogger("pbtc")
	log.Debug("Connection handler starting")

	// let the waitgroup know when we are done
	defer mgr.wg.Done()

ConnLoop:
	for {
		select {
		// this is the signal to quit, so break the outer loop
		case _, ok := <-mgr.sigConn:
			if !ok {
				log.Debug("Connection handler stopping")
				break ConnLoop
			}

		// the ticker will signal each time we can attempt a new connection
		// if we don't have too many peers yet, try to create a new one
		case <-mgr.connTicker.C:
			if len(mgr.peerIndex) < maxPeerCount {
				mgr.addPeer()
			}
		}
	}

	log.Debug("Connection handler stopped")
}

// handlePeers will execute householding operations on new peers and peers
// that have expired. It should be used to keep track of peers and to convey
// application state to the peers.
func (mgr *Manager) handlePeers() {
	log := logging.MustGetLogger("pbtc")
	log.Debug("Peer handler starting")

	// let the waitgroup know when we are done
	defer mgr.wg.Done()

PeerLoop:
	for {
		select {
		// this is the signal to quit, so break the outer loop
		case _, ok := <-mgr.sigPeer:
			if !ok {
				log.Debug("Peer handler stopping")
				break PeerLoop
			}

		// whenever there is a new peer to be added, process it
		case peer := <-mgr.peerNew:
			log.Debug("Peer handler processing new peer")
			mgr.processNewPeer(peer)

		// whenever there is an expired peer to be removed, process it
		case peer := <-mgr.peerDone:
			log.Debug("Peer handler processing done peer")
			mgr.processDonePeer(peer)
		}
	}

	log.Debug("Peer handler stopped")
}

// processListener is a dedicated loop to be run for every local IP that we
// want to listen on. It should be run as a go routine and will try accepting
// new connections.
func (mgr *Manager) handleListener(listener *net.TCPListener) {
	log := logging.MustGetLogger("pbtc")
	log.Debug("Listener handler starting for %v", listener.Addr())

	// let the waitgroup know when we are done
	defer mgr.wg.Done()

	for {
		// try accepting a new connection
		conn, err := listener.AcceptTCP()
		// this is ugly, but the listener does not follow the convention of
		// returning an io.EOF error, but rather an unexported one
		// we need to treat it separately to keep the logs clean, as this
		// is how we do a clean and voluntary shutdown of these handlers
		if strings.Contains(err.Error(), "use of closed network connection") {
			log.Debug("Listen handler stopping for %v", listener.Addr())
			break
		}
		if err != nil {
			log.Warning("Could not accept connection: %v", err)
			break
		}

		// create a new incoming peer for the given connection
		// if the connection is valid, the peer will notify the manager on its own
		err = newIncomingPeer(mgr, conn, mgr.network, mgr.version, mgr.nonce)
		if err != nil {
			log.Error("Could not create incoming peer")
			continue
		}
	}

	log.Debug("Listener handler stopped for %v", listener.Addr())
}

// addPeer will try to connect to a new peer and start it on success.
func (mgr *Manager) addPeer() {
	log := logging.MustGetLogger("pbtc")

	tries := 0
	for {
		// if we tried too many times, give up for this time
		tries++
		if tries > maxAddrAttempts {
			log.Notice("Could not find good address")
			return
		}

		// try to get the best address from the repository
		addr, err := mgr.repo.Get()
		if err != nil {
			log.Notice("Repository could not return address: %v", err)
			return
		}

		// check if the address in still unused
		_, ok := mgr.peerIndex[addr.String()]
		if ok {
			log.Notice("Already connected to %v", addr)
			continue
		}

		// we initialize a new peer which will callback through a channel on success
		err = newOutgoingPeer(mgr, addr, mgr.network, mgr.version, mgr.nonce)
		if err != nil {
			log.Error("Could not create peer for %v", addr)
			return
		}

		log.Info("Initializing new peer for %v", addr)
		mgr.repo.Attempt(addr)
		break
	}
}

// processNewPeer is what we do with new initialized peers that are added to
// the manager. The peers should be in a connected state so we can start them
// and add them to our index.
func (mgr *Manager) processNewPeer(peer *peer) {
	log := logging.MustGetLogger("pbtc")
	_, ok := mgr.peerIndex[peer.String()]
	if ok {
		log.Warning("Trying to add existing peer to index %v", peer)
		return
	}

	if len(mgr.peerIndex) >= maxPeerCount {
		log.Notice("Maximum peer number reached, discarding peer %v", peer)
		return
	}

	log.Debug("Starting new peer %v", peer)
	peer.Start()
	mgr.peerIndex[peer.String()] = peer
}

// processDonePeer is what we do to expired peers. They failed in some way and
// already initialized shutdown on their own, so we just need to remove them
// from our index.
func (mgr *Manager) processDonePeer(peer *peer) {
	log := logging.MustGetLogger("pbtc")
	_, ok := mgr.peerIndex[peer.String()]
	if !ok {
		log.Notice("Done peer already removed from peer index")
		return
	}

	log.Debug("Removing done peer from peer index")
	delete(mgr.peerIndex, peer.String())
}
