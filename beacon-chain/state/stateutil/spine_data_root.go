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

package stateutil

import (
	"encoding/binary"

	"github.com/pkg/errors"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/bytesutil"
	"gitlab.waterfall.network/waterfall/protocol/coordinator/encoding/ssz"
	ethpb "gitlab.waterfall.network/waterfall/protocol/coordinator/proto/prysm/v1alpha1"
)

// SpineDataRootWithHasher returns the hash tree root of input `spineData`.
func SpineDataRootWithHasher(spineData *ethpb.SpineData) ([32]byte, error) {
	if spineData == nil {
		return [32]byte{}, errors.New("nil spine data")
	}

	spinesRoot, err := BytesRoot(spineData.Spines)
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "spines root failed")
	}
	prefixRoot, err := BytesRoot(spineData.Prefix)
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "prefix root failed")
	}

	finalizationRoot, err := BytesRoot(spineData.Finalization)
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "finalization root failed")
	}

	cpFinalizedRoot, err := BytesRoot(spineData.CpFinalized)
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "cpFinalized root failed")
	}

	parentSpineRoot, err := getParentSpinesRoot(spineData.ParentSpines)
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "parent spines root failed")
	}

	fieldRoots := [][32]byte{
		spinesRoot,
		prefixRoot,
		finalizationRoot,
		cpFinalizedRoot,
		parentSpineRoot,
	}

	root, err := ssz.BitwiseMerkleize(fieldRoots, uint64(len(fieldRoots)), uint64(len(fieldRoots)))
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "spines data merkleize failed")
	}
	return root, nil
}

func BytesRoot(bts []byte) ([32]byte, error) {
	btsLen := len(bts)
	btsChunksLen := (btsLen / 32)
	if btsLen%32 > 0 {
		btsChunksLen++
	}
	btsChunks := make([][32]byte, btsChunksLen)
	for i := 0; i < btsChunksLen; i++ {
		from := i * 32
		to := from + 32
		if to > btsLen {
			to = btsLen
		}
		val := bts[from:to]
		if len(val) > 0 {
			btsChunks[i] = bytesutil.ToBytes32(val)
		} else {
			btsChunks[i] = [32]byte{}
		}
	}
	btsChunksRoot, err := ssz.BitwiseMerkleize(btsChunks, uint64(len(btsChunks)), uint64(len(btsChunks)))
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "could not compute bytes root merkleization")
	}
	btsLengthRoot := make([]byte, 32)
	binary.LittleEndian.PutUint64(btsLengthRoot, uint64(len(bts)))
	return ssz.MixInLength(btsChunksRoot, btsLengthRoot), nil
}

func getParentSpinesRoot(parentSpines []*ethpb.SpinesSeq) ([32]byte, error) {
	allRoots := make([][32]byte, len(parentSpines))
	for i, ps := range parentSpines {
		root, err := BytesRoot(ps.Spines)
		if err != nil {
			return [32]byte{}, errors.Wrap(err, "parent spines root failed")
		}
		allRoots[i] = root
	}

	rootsRoot, err := ssz.BitwiseMerkleize(allRoots, uint64(len(allRoots)), uint64(len(allRoots)))
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "could not compute bytes root ops merkleization")
	}
	btsLengthRoot := make([]byte, 32)
	binary.LittleEndian.PutUint64(btsLengthRoot, uint64(len(allRoots)))
	return ssz.MixInLength(rootsRoot, btsLengthRoot), nil
}
