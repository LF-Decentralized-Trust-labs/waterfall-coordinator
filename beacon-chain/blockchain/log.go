package blockchain

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/config/params"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/block"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/runtime/version"
	prysmTime "gitlab.waterfall.network/waterfall/protocol/coordinator/time"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/time/slots"
)

var log = logrus.WithField("prefix", "blockchain")

// logs state transition related data every slot.
func logStateTransitionData(b block.BeaconBlock) error {
	log := log.WithField("slot", b.Slot())
	if len(b.Body().Attestations()) > 0 {
		log = log.WithField("attestations", len(b.Body().Attestations()))
	}
	if len(b.Body().Deposits()) > 0 {
		log = log.WithField("deposits", len(b.Body().Deposits()))
	}
	if len(b.Body().AttesterSlashings()) > 0 {
		log = log.WithField("attesterSlashings", len(b.Body().AttesterSlashings()))
	}
	if len(b.Body().ProposerSlashings()) > 0 {
		log = log.WithField("proposerSlashings", len(b.Body().ProposerSlashings()))
	}
	if len(b.Body().VoluntaryExits()) > 0 {
		log = log.WithField("voluntaryExits", len(b.Body().VoluntaryExits()))
	}
	if len(b.Body().Withdrawals()) > 0 {
		log = log.WithField("withdrawals", len(b.Body().Withdrawals()))
	}
	if b.Version() == version.Altair {
		agg, err := b.Body().SyncAggregate()
		if err != nil {
			return err
		}
		log = log.WithField("syncBitsCount", agg.SyncCommitteeBits.Count())
	}
	log.Info("Finished applying state transition")
	return nil
}

func logBlockSyncStatus(block block.BeaconBlock, blockRoot [32]byte, finalized *ethpb.Checkpoint, receivedTime time.Time, genesisTime uint64) error {
	startTime, err := slots.ToTime(genesisTime, block.Slot())
	if err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		"slot":           block.Slot(),
		"slotInEpoch":    block.Slot() % params.BeaconConfig().SlotsPerEpoch,
		"block":          fmt.Sprintf("0x%s...", hex.EncodeToString(blockRoot[:])[:8]),
		"epoch":          slots.ToEpoch(block.Slot()),
		"finalizedEpoch": finalized.Epoch,
		"finalizedRoot":  fmt.Sprintf("0x%s...", hex.EncodeToString(finalized.Root)[:8]),
		"parentRoot":     fmt.Sprintf("0x%s...", hex.EncodeToString(block.ParentRoot())[:8]),
		"version":        version.String(block.Version()),
	}).Info("Synced new block")
	log.WithFields(logrus.Fields{
		"slot":                      block.Slot,
		"sinceSlotStartTime":        prysmTime.Now().Sub(startTime),
		"chainServiceProcessedTime": prysmTime.Now().Sub(receivedTime),
	}).Debug("Sync new block times")
	return nil
}
