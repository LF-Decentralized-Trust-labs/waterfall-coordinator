//Copyright 2024   Blue Wave Inc.
//
//Licensed under the Apache License, Version 2.0 (the "License");
//you may not use this file except in compliance with the License.
//You may obtain a copy of the License at
//
//http://www.apache.org/licenses/LICENSE-2.0
//
//Unless required by applicable law or agreed to in writing, software
//distributed under the License is distributed on an "AS IS" BASIS,
//WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//See the License for the specific language governing permissions and
//limitations under the License.

package v1

import (
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
)

// SpineData obtain spine information stored in the beacon state.
func (b *BeaconState) SpineData() *ethpb.SpineData {
	if b.spineData == nil {
		return nil
	}
	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.spineDataVal()
}

// spineDataVal corresponding to spine information stored in the beacon state.
// This assumes that a lock is already held on BeaconState.
func (b *BeaconState) spineDataVal() *ethpb.SpineData {
	if b.spineData == nil {
		return nil
	}

	return ethpb.CopySpineData(b.spineData)
}
