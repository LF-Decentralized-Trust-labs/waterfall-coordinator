package stategen

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/beacon-chain/state"
	"go.opencensus.io/trace"
)

// MigrateToCold advances the finalized info in between the cold and hot state sections.
// It moves the recent finalized states from the hot section to the cold section and
// only preserve the ones that's on archived point.
func (s *State) MigrateToCold(ctx context.Context, fRoot [32]byte) error {
	ctx, span := trace.StartSpan(ctx, "stateGen.MigrateToCold")
	defer span.End()

	// skip if another process running
	if s.isMigratingToCold.Load() {
		log.Warn("MigrateToCold: skipping: already running")
		return nil
	}
	s.isMigratingToCold.Store(true)
	defer s.isMigratingToCold.Store(false)

	s.finalizedInfo.lock.RLock()
	oldFSlot := s.finalizedInfo.slot
	s.finalizedInfo.lock.RUnlock()

	fBlock, err := s.beaconDB.Block(ctx, fRoot)
	if err != nil {
		return err
	}
	fSlot := fBlock.Block().Slot()
	if oldFSlot > fSlot {
		return nil
	}

	// Start at previous finalized slot, stop at current finalized slot.
	// If the slot is on archived point, save the state of that slot to the DB.
	for slot := oldFSlot; slot < fSlot; slot++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if slot%s.slotsPerArchivedPoint == 0 && slot != 0 {
			cached, exists, err := s.epochBoundaryStateCache.getBySlot(slot)
			if err != nil {
				return fmt.Errorf("could not get epoch boundary state for slot %d", slot)
			}

			var aRoot [32]byte
			var aState state.BeaconState

			// When the epoch boundary state is not in cache due to skip slot scenario,
			// we have to regenerate the state which will represent epoch boundary.
			// By finding the highest available block below epoch boundary slot, we
			// generate the state for that block root.
			if exists {
				aRoot = cached.root
				aState = cached.state
			} else {

				log.WithFields(logrus.Fields{
					"slot": slot,
				}).Info("MigrateToCold: no cached state 000")
				checkTime := time.Now()

				blks, err := s.beaconDB.HighestSlotBlocksBelow(ctx, slot)
				if err != nil {
					return err
				}

				log.WithFields(logrus.Fields{
					"slot":    slot,
					"elapsed": time.Since(checkTime),
				}).Info("MigrateToCold: no cached state 111")
				checkTime = time.Now()

				// Given the block has been finalized, the db should not have more than one block in a given slot.
				// We should error out when this happens.
				if len(blks) != 1 {
					return errUnknownBlock
				}
				missingRoot, err := blks[0].Block().HashTreeRoot()
				if err != nil {
					return err
				}
				aRoot = missingRoot
				// There's no need to generate the state if the state already exists on the DB.
				// We can skip saving the state.
				if !s.beaconDB.HasState(ctx, aRoot) {

					log.WithFields(logrus.Fields{
						"slot":     slot,
						"aRoot":    fmt.Sprintf("%#x", aRoot),
						"HasState": false,
						"elapsed":  time.Since(checkTime),
					}).Info("MigrateToCold: no cached state 222 (no state)")
					checkTime = time.Now()

					aState, err = s.StateByRoot(ctx, missingRoot)
					if err != nil {
						return err
					}

					log.WithFields(logrus.Fields{
						"slot":     slot,
						"aRoot":    fmt.Sprintf("%#x", aRoot),
						"HasState": false,
						"elapsed":  time.Since(checkTime),
					}).Info("MigrateToCold: no cached state 333 (no state)")
				}
				log.WithFields(logrus.Fields{
					"slot":     slot,
					"aRoot":    fmt.Sprintf("%#x", aRoot),
					"HasState": true,
					"elapsed":  time.Since(checkTime),
				}).Info("MigrateToCold: no cached state 222 (has state)")
			}

			if s.beaconDB.HasState(ctx, aRoot) {
				// Remove hot state DB root to prevent it gets deleted later when we turn hot state save DB mode off.
				s.saveHotStateDB.lock.Lock()
				roots := s.saveHotStateDB.savedStateRoots
				for i := 0; i < len(roots); i++ {
					if aRoot == roots[i] {
						s.saveHotStateDB.savedStateRoots = append(roots[:i], roots[i+1:]...)
						// There shouldn't be duplicated roots in `savedStateRoots`.
						// Break here is ok.
						break
					}
				}
				s.saveHotStateDB.lock.Unlock()
				continue
			}

			if err := s.beaconDB.SaveState(ctx, aState, aRoot); err != nil {
				return err
			}
			log.WithFields(logrus.Fields{
				"slot": aState.Slot(),
				"root": fmt.Sprintf("%#x", aRoot), //hex.EncodeToString(bytesutil.Trunc(aRoot[:])),
			}).Info("Saved state in DB")
		}
	}

	// Update finalized info in memory.
	fInfo, ok, err := s.epochBoundaryStateCache.getByRoot(fRoot)
	if err != nil {
		return err
	}
	if ok {
		s.SaveFinalizedState(fSlot, fRoot, fInfo.state)
		//if err := s.beaconDB.SaveState(ctx, fInfo.state, fRoot); err != nil {
		//	return err
		//}
		//log.WithFields(logrus.Fields{
		//	"slot": fInfo.state.Slot(),
		//	"root": fmt.Sprintf("%#x", fRoot),
		//}).Info("Saved state of fin cp in DB")
	}

	return nil
}
