package sync

import (
	"gitlab.waterfall.network/waterfall/protocol/coordinator/async/event"
	blockfeed "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/feed/block"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/feed/operation"
	statefeed "gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/core/feed/state"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/db"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/operations/attestations"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/operations/prevote"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/operations/slashings"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/operations/synccommittee"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/operations/voluntaryexits"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/operations/withdrawals"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/p2p"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state/stategen"
)

// Option defines options to initilize service.
type Option func(s *Service) error

// WithAttestationNotifier apply Notifier option while initialize service.
func WithAttestationNotifier(notifier operation.Notifier) Option {
	return func(s *Service) error {
		s.cfg.attestationNotifier = notifier
		return nil
	}
}

// WithP2P apply P2P option while initialize service.
func WithP2P(p2p p2p.P2P) Option {
	return func(s *Service) error {
		s.cfg.p2p = p2p
		return nil
	}
}

// WithDatabase apply DB option while initialize service.
func WithDatabase(db db.NoHeadAccessDatabase) Option {
	return func(s *Service) error {
		s.cfg.beaconDB = db
		return nil
	}
}

// WithAttestationPool apply attPool option while initialize service.
func WithAttestationPool(attPool attestations.Pool) Option {
	return func(s *Service) error {
		s.cfg.attPool = attPool
		return nil
	}
}

// WithPrevotePool apply PrevotePool option while initialize service.
func WithPrevotePool(pvPool prevote.Pool) Option {
	return func(s *Service) error {
		s.cfg.prevotePool = pvPool
		return nil
	}
}

// WithExitPool apply  option exitPool while initialize service.
func WithExitPool(exitPool voluntaryexits.PoolManager) Option {
	return func(s *Service) error {
		s.cfg.exitPool = exitPool
		return nil
	}
}

// WithWithdrawalPool apply withWithdrawalPool option while initialize service.
func WithWithdrawalPool(withdrawalPool withdrawals.PoolManager) Option {
	return func(s *Service) error {
		s.cfg.withdrawalPool = withdrawalPool
		return nil
	}
}

// WithSlashingPool apply SlashingPool option while initialize service.
func WithSlashingPool(slashingPool slashings.PoolManager) Option {
	return func(s *Service) error {
		s.cfg.slashingPool = slashingPool
		return nil
	}
}

// WithSyncCommsPool apply syncCommsPool option while initialize service.
func WithSyncCommsPool(syncCommsPool synccommittee.Pool) Option {
	return func(s *Service) error {
		s.cfg.syncCommsPool = syncCommsPool
		return nil
	}
}

// WithChainService apply ChainService option while initialize service.
func WithChainService(chain blockchainService) Option {
	return func(s *Service) error {
		s.cfg.chain = chain
		return nil
	}
}

// WithInitialSync apply initialSync option while initialize service.
func WithInitialSync(initialSync Checker) Option {
	return func(s *Service) error {
		s.cfg.initialSync = initialSync
		return nil
	}
}

// WithStateNotifier apply StateNotifier option while initialize service.
func WithStateNotifier(stateNotifier statefeed.Notifier) Option {
	return func(s *Service) error {
		s.cfg.stateNotifier = stateNotifier
		return nil
	}
}

// WithBlockNotifier apply BlockNotifier option while initialize service.
func WithBlockNotifier(blockNotifier blockfeed.Notifier) Option {
	return func(s *Service) error {
		s.cfg.blockNotifier = blockNotifier
		return nil
	}
}

// WithOperationNotifier apply OperationNotifier option while initialize service.
func WithOperationNotifier(operationNotifier operation.Notifier) Option {
	return func(s *Service) error {
		s.cfg.operationNotifier = operationNotifier
		return nil
	}
}

// WithStateGen apply StateGen option while initialize service.
func WithStateGen(stateGen *stategen.State) Option {
	return func(s *Service) error {
		s.cfg.stateGen = stateGen
		return nil
	}
}

// WithSlasherAttestationsFeed apply SlasherAttestationsFeed option while initialize service.
func WithSlasherAttestationsFeed(slasherAttestationsFeed *event.Feed) Option {
	return func(s *Service) error {
		s.cfg.slasherAttestationsFeed = slasherAttestationsFeed
		return nil
	}
}

// WithSlasherBlockHeadersFeed apply SlasherBlockHeadersFeed option while initialize service.
func WithSlasherBlockHeadersFeed(slasherBlockHeadersFeed *event.Feed) Option {
	return func(s *Service) error {
		s.cfg.slasherBlockHeadersFeed = slasherBlockHeadersFeed
		return nil
	}
}
