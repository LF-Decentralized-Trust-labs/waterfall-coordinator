package p2p

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/rand"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kevinms/leakybucket-go"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/prysmaticlabs/go-bitfield"
	logTest "github.com/sirupsen/logrus/hooks/test"
	mock "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/blockchain/testing"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/cache"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/feed"
	statefeed "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/feed/state"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/p2p/peers"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/p2p/peers/peerdata"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/p2p/peers/scorers"
	testp2p "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/p2p/testing"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	prysmNetwork "gitlab.waterfall.network/waterfall/protocol/coordinator/network"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/wrapper"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/runtime/version"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/assert"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/require"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/time/slots"
	"gitlab.waterfall.network/waterfall/protocol/gwat/p2p/discover"
	"gitlab.waterfall.network/waterfall/protocol/gwat/p2p/enode"
	"gitlab.waterfall.network/waterfall/protocol/gwat/p2p/enr"
)

var discoveryWaitTime = 1 * time.Second

func init() {
	rand.Seed(time.Now().Unix())
}

func createAddrAndPrivKey(t *testing.T) (net.IP, *ecdsa.PrivateKey) {
	ip, err := prysmNetwork.ExternalIPv4()
	require.NoError(t, err, "Could not get ip")
	ipAddr := net.ParseIP(ip)
	temp := t.TempDir()
	randNum := rand.Int()
	tempPath := path.Join(temp, strconv.Itoa(randNum))
	require.NoError(t, os.Mkdir(tempPath, 0700))
	pkey, err := privKey(&Config{DataDir: tempPath})
	require.NoError(t, err, "Could not get private key")
	return ipAddr, pkey
}

func TestCreateListener(t *testing.T) {
	port := 1024
	ipAddr, pkey := createAddrAndPrivKey(t)
	s := &Service{
		started:               true,
		genesisTime:           time.Now(),
		genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
		cfg:                   &Config{UDPPort: uint(port)},
	}
	listener, err := s.createListener(ipAddr, pkey)
	require.NoError(t, err)
	defer listener.Close()

	assert.Equal(t, true, listener.Self().IP().Equal(ipAddr), "IP address is not the expected type")
	assert.Equal(t, port, listener.Self().UDP(), "Incorrect port number")

	pubkey := listener.Self().Pubkey()
	XisSame := pkey.PublicKey.X.Cmp(pubkey.X) == 0
	YisSame := pkey.PublicKey.Y.Cmp(pubkey.Y) == 0

	if !(XisSame && YisSame) {
		t.Error("Pubkey is different from what was used to create the listener")
	}
}

func TestStartDiscV5_DiscoverAllPeers(t *testing.T) {
	port := 2000
	ipAddr, pkey := createAddrAndPrivKey(t)
	genesisTime := time.Now()
	genesisValidatorsRoot := make([]byte, 32)
	s := &Service{
		started:               true,
		cfg:                   &Config{UDPPort: uint(port)},
		genesisTime:           genesisTime,
		genesisValidatorsRoot: genesisValidatorsRoot,
	}
	bootListener, err := s.createListener(ipAddr, pkey)
	require.NoError(t, err)
	defer bootListener.Close()

	bootNode := bootListener.Self()

	var listeners []*discover.UDPv5
	for i := 1; i <= 5; i++ {
		port = 3000 + i
		cfg := &Config{
			Discv5BootStrapAddr: []string{bootNode.String()},
			UDPPort:             uint(port),
		}
		ipAddr, pkey := createAddrAndPrivKey(t)
		s = &Service{
			cfg:                   cfg,
			genesisTime:           genesisTime,
			genesisValidatorsRoot: genesisValidatorsRoot,
		}
		listener, err := s.startDiscoveryV5(ipAddr, pkey)
		assert.NoError(t, err, "Could not start discovery for node")
		listeners = append(listeners, listener)
	}
	defer func() {
		// Close down all peers.
		for _, listener := range listeners {
			listener.Close()
		}
	}()

	// Wait for the nodes to have their local routing tables to be populated with the other nodes
	time.Sleep(discoveryWaitTime)

	lastListener := listeners[len(listeners)-1]
	nodes := lastListener.Lookup(bootNode.ID())
	if len(nodes) < 4 {
		t.Errorf("The node's local table doesn't have the expected number of nodes. "+
			"Expected more than or equal to %d but got %d", 4, len(nodes))
	}
}

func TestCreateLocalNode(t *testing.T) {
	testCases := []struct {
		name          string
		cfg           *Config
		expectedError bool
	}{
		{
			name:          "valid config",
			cfg:           nil,
			expectedError: false,
		},
		{
			name:          "invalid host address",
			cfg:           &Config{HostAddress: "invalid"},
			expectedError: true,
		},
		{
			name:          "valid host address",
			cfg:           &Config{HostAddress: "192.168.0.1"},
			expectedError: false,
		},
		{
			name:          "invalid host DNS",
			cfg:           &Config{HostDNS: "invalid"},
			expectedError: true,
		},
		// // fail bazel test (but bazel debug & go: success)
		//{
		//	name:          "valid host DNS",
		//	cfg:           &Config{HostDNS: "google.com"},
		//	expectedError: false,
		//},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			// Define ports.
			const (
				udpPort = 2000
				tcpPort = 3000
			)

			// Create a private key.
			address, privKey := createAddrAndPrivKey(t)

			// Create a service.
			service := &Service{
				genesisTime:           time.Now(),
				genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
				cfg:                   tt.cfg,
			}

			localNode, err := service.createLocalNode(privKey, address, udpPort, tcpPort)
			if tt.expectedError {
				require.NotNil(t, err)
				return
			}

			require.NoError(t, err)

			expectedAddress := address
			if tt.cfg != nil && tt.cfg.HostAddress != "" {
				expectedAddress = net.ParseIP(tt.cfg.HostAddress)
			}

			// Check IP.
			// IP is not checked int case of DNS, since it can be resolved to different IPs.
			if tt.cfg == nil || tt.cfg.HostDNS == "" {
				ip := new(net.IP)
				require.NoError(t, localNode.Node().Record().Load(enr.WithEntry("ip", ip)))
				require.Equal(t, true, ip.Equal(expectedAddress))
				require.Equal(t, true, localNode.Node().IP().Equal(expectedAddress))
			}

			// Check UDP.
			udp := new(uint16)
			require.NoError(t, localNode.Node().Record().Load(enr.WithEntry("udp", udp)))
			require.Equal(t, udpPort, localNode.Node().UDP())

			// Check TCP.
			tcp := new(uint16)
			require.NoError(t, localNode.Node().Record().Load(enr.WithEntry("tcp", tcp)))
			require.Equal(t, tcpPort, localNode.Node().TCP())

			// Check fork is set.
			fork := new([]byte)
			require.NoError(t, localNode.Node().Record().Load(enr.WithEntry(eth2ENRKey, fork)))
			require.NotEmpty(t, *fork)

			// Check att subnets.
			attSubnets := new([]byte)
			require.NoError(t, localNode.Node().Record().Load(enr.WithEntry(attSubnetEnrKey, attSubnets)))
			require.DeepSSZEqual(t, []byte{0, 0, 0, 0, 0, 0, 0, 0}, *attSubnets)

			// Check sync committees subnets.
			syncSubnets := new([]byte)
			require.NoError(t, localNode.Node().Record().Load(enr.WithEntry(syncCommsSubnetEnrKey, syncSubnets)))
			require.DeepSSZEqual(t, []byte{0}, *syncSubnets)
		})
	}
}

func TestMultiAddrsConversion_InvalidIPAddr(t *testing.T) {
	addr := net.ParseIP("invalidIP")
	_, pkey := createAddrAndPrivKey(t)
	s := &Service{
		started:               true,
		genesisTime:           time.Now(),
		genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
	}
	node, err := s.createLocalNode(pkey, addr, 0, 0)
	require.NoError(t, err)
	multiAddr := convertToMultiAddr([]*enode.Node{node.Node()})
	assert.Equal(t, 0, len(multiAddr), "Invalid ip address converted successfully")
}

func TestMultiAddrConversion_OK(t *testing.T) {
	hook := logTest.NewGlobal()
	ipAddr, pkey := createAddrAndPrivKey(t)
	s := &Service{
		started: true,
		cfg: &Config{
			TCPPort: 0,
			UDPPort: 0,
		},
		genesisTime:           time.Now(),
		genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
	}
	listener, err := s.createListener(ipAddr, pkey)
	require.NoError(t, err)
	defer listener.Close()

	_ = convertToMultiAddr([]*enode.Node{listener.Self()})
	require.LogsDoNotContain(t, hook, "Node doesn't have an ip4 address")
	require.LogsDoNotContain(t, hook, "Invalid port, the tcp port of the node is a reserved port")
	require.LogsDoNotContain(t, hook, "Could not get multiaddr")
}

func TestStaticPeering_PeersAreAdded(t *testing.T) {
	cfg := &Config{
		MaxPeers: 30,
	}
	port := 6000
	var staticPeers []string
	var hosts []host.Host
	// setup other nodes
	for i := 1; i <= 5; i++ {
		h, _, ipaddr := createHost(t, port+i)
		staticPeers = append(staticPeers, fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", ipaddr, port+i, h.ID()))
		hosts = append(hosts, h)
	}

	defer func() {
		for _, h := range hosts {
			if err := h.Close(); err != nil {
				t.Log(err)
			}
		}
	}()

	cfg.TCPPort = 14500
	cfg.UDPPort = 14501
	cfg.StaticPeers = staticPeers
	cfg.StateNotifier = &mock.MockStateNotifier{}
	cfg.NoDiscovery = true
	s, err := NewService(context.Background(), cfg)
	require.NoError(t, err)

	exitRoutine := make(chan bool)
	go func() {
		s.Start()
		<-exitRoutine
	}()
	time.Sleep(50 * time.Millisecond)
	// Send in a loop to ensure it is delivered (busy wait for the service to subscribe to the state feed).
	for sent := 0; sent == 0; {
		sent = s.stateNotifier.StateFeed().Send(&feed.Event{
			Type: statefeed.Initialized,
			Data: &statefeed.InitializedData{
				StartTime:             time.Now(),
				GenesisValidatorsRoot: make([]byte, 32),
			},
		})
	}
	time.Sleep(4 * time.Second)
	peers := s.host.Network().Peers()
	assert.Equal(t, 5, len(peers), "Not all peers added to peerstore")
	require.NoError(t, s.Stop())
	exitRoutine <- true
}

func TestHostIsResolved(t *testing.T) {
	// As defined in RFC 2606 , example.org is a
	// reserved example domain name.
	exampleHost := "example.org"
	exampleIP := "93.184.216.34"

	s := &Service{
		started: true,
		cfg: &Config{
			HostDNS: exampleHost,
		},
		genesisTime:           time.Now(),
		genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
	}
	ip, key := createAddrAndPrivKey(t)
	list, err := s.createListener(ip, key)
	require.NoError(t, err)

	newIP := list.Self().IP()
	assert.Equal(t, exampleIP, newIP.String(), "Did not resolve to expected IP")
}

func TestInboundPeerLimit(t *testing.T) {
	fakePeer := testp2p.NewTestP2P(t)
	s := &Service{
		started:   true,
		cfg:       &Config{MaxPeers: 30},
		ipLimiter: leakybucket.NewCollector(ipLimit, ipBurst, false),
		peers: peers.NewStatus(context.Background(), &peers.StatusConfig{
			PeerLimit:    30,
			ScorerParams: &scorers.Config{},
		}),
		host: fakePeer.BHost,
	}

	for i := 0; i < 30; i++ {
		_ = addPeer(t, s.peers, peerdata.PeerConnectionState(ethpb.ConnectionState_CONNECTED))
	}

	require.Equal(t, true, s.isPeerAtLimit(false), "not at limit for outbound peers")
	require.Equal(t, false, s.isPeerAtLimit(true), "at limit for inbound peers")

	for i := 0; i < highWatermarkBuffer; i++ {
		_ = addPeer(t, s.peers, peerdata.PeerConnectionState(ethpb.ConnectionState_CONNECTED))
	}

	require.Equal(t, true, s.isPeerAtLimit(true), "not at limit for inbound peers")
}

func TestUDPMultiAddress(t *testing.T) {
	port := 6500
	ipAddr, pkey := createAddrAndPrivKey(t)
	genesisTime := time.Now()
	genesisValidatorsRoot := make([]byte, 32)
	s := &Service{
		started:               true,
		cfg:                   &Config{UDPPort: uint(port)},
		genesisTime:           genesisTime,
		genesisValidatorsRoot: genesisValidatorsRoot,
	}
	listener, err := s.createListener(ipAddr, pkey)
	require.NoError(t, err)
	defer listener.Close()
	s.dv5Listener = listener

	multiAddresses, err := s.DiscoveryAddresses()
	require.NoError(t, err)
	require.Equal(t, true, len(multiAddresses) > 0)
	assert.Equal(t, true, strings.Contains(multiAddresses[0].String(), fmt.Sprintf("%d", port)))
	assert.Equal(t, true, strings.Contains(multiAddresses[0].String(), "udp"))
}

func TestMultipleDiscoveryAddresses(t *testing.T) {
	db, err := enode.OpenDB(t.TempDir())
	require.NoError(t, err)
	_, key := createAddrAndPrivKey(t)
	node := enode.NewLocalNode(db, key)
	node.Set(enr.IPv4{127, 0, 0, 1})
	node.Set(enr.IPv6{0x20, 0x01, 0x48, 0x60, 0, 0, 0x20, 0x01, 0, 0, 0, 0, 0, 0, 0x00, 0x68})
	s := &Service{dv5Listener: mockListener{localNode: node}}

	multiAddresses, err := s.DiscoveryAddresses()
	require.NoError(t, err)
	require.Equal(t, 2, len(multiAddresses))
	ipv4Found, ipv6Found := false, false
	for _, address := range multiAddresses {
		s := address.String()
		if strings.Contains(s, "ip4") {
			ipv4Found = true
		} else if strings.Contains(s, "ip6") {
			ipv6Found = true
		}
	}
	assert.Equal(t, true, ipv4Found, "IPv4 discovery address not found")
	assert.Equal(t, true, ipv6Found, "IPv6 discovery address not found")
}

func TestCorrectUDPVersion(t *testing.T) {
	assert.Equal(t, udp4, udpVersionFromIP(net.IPv4zero), "incorrect network version")
	assert.Equal(t, udp6, udpVersionFromIP(net.IPv6zero), "incorrect network version")
	assert.Equal(t, udp4, udpVersionFromIP(net.IP{200, 20, 12, 255}), "incorrect network version")
	assert.Equal(t, udp6, udpVersionFromIP(net.IP{22, 23, 24, 251, 17, 18, 0, 0, 0, 0, 12, 14, 212, 213, 16, 22}), "incorrect network version")
	// v4 in v6
	assert.Equal(t, udp4, udpVersionFromIP(net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 212, 213, 16, 22}), "incorrect network version")
}

// addPeer is a helper to add a peer with a given connection state)
func addPeer(t *testing.T, p *peers.Status, state peerdata.PeerConnectionState) peer.ID {
	// Set up some peers with different states
	mhBytes := []byte{0x11, 0x04}
	idBytes := make([]byte, 4)
	_, err := rand.Read(idBytes)
	require.NoError(t, err)
	mhBytes = append(mhBytes, idBytes...)
	id, err := peer.IDFromBytes(mhBytes)
	require.NoError(t, err)
	p.Add(new(enr.Record), id, nil, network.DirInbound)
	p.SetConnectionState(id, state)
	p.SetMetadata(id, wrapper.WrappedMetadataV0(&ethpb.MetaDataV0{
		SeqNumber: 0,
		Attnets:   bitfield.NewBitvector64(),
	}))
	return id
}

func TestRefreshENR_ForkBoundaries(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	// Clean up caches after usage.
	defer cache.SubnetIDs.EmptyAllCaches()

	tests := []struct {
		name           string
		svcBuilder     func(t *testing.T) *Service
		postValidation func(t *testing.T, s *Service)
	}{
		{
			name: "metadata no change",
			svcBuilder: func(t *testing.T) *Service {
				port := 2000
				ipAddr, pkey := createAddrAndPrivKey(t)
				s := &Service{
					started:               true,
					genesisTime:           time.Now(),
					genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
					cfg:                   &Config{UDPPort: uint(port)},
				}
				listener, err := s.createListener(ipAddr, pkey)
				assert.NoError(t, err)
				s.dv5Listener = listener
				s.metaData = wrapper.WrappedMetadataV0(new(ethpb.MetaDataV0))
				s.updateSubnetRecordWithMetadata([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
				return s
			},
			postValidation: func(t *testing.T, s *Service) {
				currEpoch := slots.ToEpoch(slots.CurrentSlot(uint64(s.genesisTime.Unix())))
				subs, err := computeSubscribedSubnets(s.dv5Listener.LocalNode().ID(), currEpoch)
				assert.NoError(t, err)

				bitV := bitfield.NewBitvector64()
				for _, idx := range subs {
					bitV.SetBitAt(idx, true)
				}
				assert.DeepEqual(t, bitV, s.metaData.AttnetsBitfield())
			},
		},
		{
			name: "metadata updated",
			svcBuilder: func(t *testing.T) *Service {
				port := 2000
				ipAddr, pkey := createAddrAndPrivKey(t)
				s := &Service{
					started:               true,
					genesisTime:           time.Now(),
					genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
					cfg:                   &Config{UDPPort: uint(port)},
				}
				listener, err := s.createListener(ipAddr, pkey)
				assert.NoError(t, err)
				s.dv5Listener = listener
				s.metaData = wrapper.WrappedMetadataV0(new(ethpb.MetaDataV0))
				s.updateSubnetRecordWithMetadata([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01})
				cache.SubnetIDs.AddPersistentCommittee([]uint64{1, 2, 3, 23}, 0)
				return s
			},
			postValidation: func(t *testing.T, s *Service) {
				assert.DeepEqual(t, bitfield.Bitvector64{0xe, 0x0, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0}, s.metaData.AttnetsBitfield())
			},
		},
		{
			name: "metadata updated at fork epoch",
			svcBuilder: func(t *testing.T) *Service {
				port := 2000
				ipAddr, pkey := createAddrAndPrivKey(t)
				s := &Service{
					started:               true,
					genesisTime:           time.Now().Add(-5 * oneEpochDuration()),
					genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
					cfg:                   &Config{UDPPort: uint(port)},
				}
				listener, err := s.createListener(ipAddr, pkey)
				assert.NoError(t, err)

				// Update params
				cfg := params.BeaconConfig().Copy()
				cfg.AltairForkEpoch = 5
				params.OverrideBeaconConfig(cfg)
				params.BeaconConfig().InitializeForkSchedule()

				s.dv5Listener = listener
				s.metaData = wrapper.WrappedMetadataV0(new(ethpb.MetaDataV0))
				s.updateSubnetRecordWithMetadata([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01})
				cache.SubnetIDs.AddPersistentCommittee([]uint64{1, 2, 3, 23}, 0)
				return s
			},
			postValidation: func(t *testing.T, s *Service) {
				assert.Equal(t, version.Altair, s.metaData.Version())
				assert.DeepEqual(t, bitfield.Bitvector4{0x00}, s.metaData.MetadataObjV1().Syncnets)
				assert.DeepEqual(t, bitfield.Bitvector64{0xe, 0x0, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0}, s.metaData.AttnetsBitfield())
			},
		},
		{
			name: "metadata updated at fork epoch with no bitfield",
			svcBuilder: func(t *testing.T) *Service {
				port := 2000
				ipAddr, pkey := createAddrAndPrivKey(t)
				s := &Service{
					started:               true,
					genesisTime:           time.Now().Add(-5 * oneEpochDuration()),
					genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
					cfg:                   &Config{UDPPort: uint(port)},
				}
				listener, err := s.createListener(ipAddr, pkey)
				assert.NoError(t, err)

				// Update params
				cfg := params.BeaconConfig().Copy()
				cfg.AltairForkEpoch = 5
				params.OverrideBeaconConfig(cfg)
				params.BeaconConfig().InitializeForkSchedule()

				s.dv5Listener = listener
				s.metaData = wrapper.WrappedMetadataV0(new(ethpb.MetaDataV0))
				s.updateSubnetRecordWithMetadata([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
				return s
			},
			postValidation: func(t *testing.T, s *Service) {
				assert.Equal(t, version.Altair, s.metaData.Version())
				assert.DeepEqual(t, bitfield.Bitvector4{0x00}, s.metaData.MetadataObjV1().Syncnets)
				currEpoch := slots.ToEpoch(slots.CurrentSlot(uint64(s.genesisTime.Unix())))
				subs, err := computeSubscribedSubnets(s.dv5Listener.LocalNode().ID(), currEpoch)
				assert.NoError(t, err)

				bitV := bitfield.NewBitvector64()
				for _, idx := range subs {
					bitV.SetBitAt(idx, true)
				}
				assert.DeepEqual(t, bitV, s.metaData.AttnetsBitfield())
			},
		},
		{
			name: "metadata updated past fork epoch with bitfields",
			svcBuilder: func(t *testing.T) *Service {
				port := 2000
				ipAddr, pkey := createAddrAndPrivKey(t)
				s := &Service{
					started:               true,
					genesisTime:           time.Now().Add(-6 * oneEpochDuration()),
					genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
					cfg:                   &Config{UDPPort: uint(port)},
				}
				listener, err := s.createListener(ipAddr, pkey)
				assert.NoError(t, err)

				// Update params
				cfg := params.BeaconConfig().Copy()
				cfg.AltairForkEpoch = 5
				params.OverrideBeaconConfig(cfg)
				params.BeaconConfig().InitializeForkSchedule()

				s.dv5Listener = listener
				s.metaData = wrapper.WrappedMetadataV0(new(ethpb.MetaDataV0))
				s.updateSubnetRecordWithMetadata([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
				cache.SubnetIDs.AddPersistentCommittee([]uint64{1, 2, 3, 23}, 0)
				cache.SyncSubnetIDs.AddSyncCommitteeSubnets([]byte{'A'}, 0, []uint64{0, 1}, 0)
				return s
			},
			postValidation: func(t *testing.T, s *Service) {
				assert.Equal(t, version.Altair, s.metaData.Version())
				assert.DeepEqual(t, bitfield.Bitvector4{0x03}, s.metaData.MetadataObjV1().Syncnets)
				assert.DeepEqual(t, bitfield.Bitvector64{0xe, 0x0, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0}, s.metaData.AttnetsBitfield())
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.svcBuilder(t)
			s.RefreshENR()
			tt.postValidation(t, s)
			s.dv5Listener.Close()
			cache.SubnetIDs.EmptyAllCaches()
			cache.SyncSubnetIDs.EmptyAllCaches()
		})
	}
}
