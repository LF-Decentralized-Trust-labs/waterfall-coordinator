package monitor

import (
	"github.com/sirupsen/logrus"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1/block"
)

// processExitsFromBlock logs the event when a tracked validators' exit was included in a block
func (s *Service) processExitsFromBlock(blk block.BeaconBlock) {
	s.RLock()
	defer s.RUnlock()
	for _, exit := range blk.Body().VoluntaryExits() {
		idx := exit.ValidatorIndex
		if s.trackedIndex(idx) {
			log.WithFields(logrus.Fields{
				"ValidatorIndex": idx,
				"Slot":           blk.Slot(),
			}).Info("Voluntary exit was included")
		}
	}
}

// processExit logs the event when tracked validators' exit was processed
func (s *Service) processExit(exit *ethpb.VoluntaryExit) {
	idx := exit.ValidatorIndex
	s.RLock()
	defer s.RUnlock()
	if s.trackedIndex(idx) {
		log.WithFields(logrus.Fields{
			"ValidatorIndex": idx,
		}).Info("Voluntary exit was processed")
	}
}
