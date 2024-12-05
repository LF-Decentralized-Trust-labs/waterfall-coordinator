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

package params

import (
	"math"

	gwatParams "gitlab.waterfall.network/waterfall/protocol/gwat/params"
)

// UseTestnet8NetworkConfig uses the Testnet8 specific network config.
func UseTestnet8NetworkConfig() {
	cfg := BeaconNetworkConfig().Copy()
	cfg.BootstrapNodes = []string{
		// Prysm's bootnode
		"enr:-LG4QC0DoIv8bWBuE_ZVx9zcrDaE1HbBPuNWVpl74GoStnSPXO0B73WF5VlfDJQSqTetQ775V9PWi7Yg3Ua7igL1ucOGAYvKzQeKh2F0" +
			"dG5ldHOIAAAAAAAAAACEZXRoMpATfFTTAAAgCf__________gmlkgnY0gmlwhICMLZGJc2VjcDI1NmsxoQKi0xTSOgGw6UO9URJjAM1T" +
			"PqPfadDeuORaJ027WIjLYIN1ZHCCD6A",
		"enr:-LG4QMRUCRExHQ8kzy5CIkhj0OEErooJ9B_J73Kg_4pXMJ1pXKNxRkCFfCdQJkG9IKss9ogMULDo_TzGV-lGSqvYaumGAY9Y1N5sh2F0dG5ldHOIAAAAAAAAAACEZXRoMpATfFTTAAAgCf__________gmlkgnY0gmlwhCIvCUqJc2VjcDI1NmsxoQI4LmVNXfuCLK5k4jE2A3-OyqNia8TBqmSXPL-Y0AP4K4N1ZHCCD6E",
		"enr:-LG4QAQig7R7VBpI-dNIzBBdduGc-LVabDyFL4sbrRwqIZjocLbgpwEPtTMyTWTlzJWPAaBG6ESy1X7OP6qPMVZi5IOGAY9Y1Qxah2F0dG5ldHOIAAAAAAAAAACEZXRoMpATfFTTAAAgCf__________gmlkgnY0gmlwhCKC9tGJc2VjcDI1NmsxoQPcRFDTKouIVs3MV5Zh8d2_VcXn7OthBeZ9_jaqMyi6j4N1ZHCCD6E",
		"enr:-LG4QDeuma56RsjBUDCHI86UI9pFpIjjM1N_4YQWh-kWHhAsPtYC3fJgL5klinXzliPb_pVF3EdsE8WchS7lVuGi4RuGAY9Y1VwTh2F0dG5ldHOIAAAAAAAAAACEZXRoMpATfFTTAAAgCf__________gmlkgnY0gmlwhCPGOqeJc2VjcDI1NmsxoQITjXuZ8nk92XqFKlge-GKkImFYGTBbtQj-0m8TxcEVE4N1ZHCCD6E",
		"enr:-LG4QGchnhJ3DwDeK_Hp_LuXrytp37Se9leedNP3TDACYGOYNDQ5gsBWDnHyEcFLzCLlWtBML2DeOLtQ7z_mj7bfgsOGAY9Y1bCph2F0dG5ldHOIAAAAAAAAAACEZXRoMpATfFTTAAAgCf__________gmlkgnY0gmlwhCKwXVaJc2VjcDI1NmsxoQNHlilG0MI_no4r9_IIzd0CMGn9vEJ-cxTDizIwvWU8ZoN1ZHCCD6E",
		"enr:-LG4QETj-mAs7kyTGzz9yaCJT5myLiE4NNJQZzP2tnWDZlGHRtg3_TDpAuhiBkuMmGGEXXgo8ft3JrqkTVk1f7aGu6GGAY9Y1erDh2F0dG5ldHOIAAAAAAAAAACEZXRoMpATfFTTAAAgCf__________gmlkgnY0gmlwhCISUpWJc2VjcDI1NmsxoQOvR7-BgTF5J1fqMOgQkIamS8YrdxZmz6LbqiQq5q0pJ4N1ZHCCD6E",
		"enr:-LG4QAUqVPZEzwfOI2OdS59VG992nUbqOBV2sSX1bNUuThbGOl7hAsrUQNEY77yFWeE5Q4uhOo98aQMiwaYU7TfM1mOGAY9Y1hZZh2F0dG5ldHOIAAAAAAAAAACEZXRoMpATfFTTAAAgCf__________gmlkgnY0gmlwhCKmKCuJc2VjcDI1NmsxoQKRLBhFnewSzUWQx9XqrZX0yUfH76D7yVQPD5al6puVGoN1ZHCCD6E",
		"enr:-LG4QOmMIlguGkU4SaCjySVY1jDWtXWE_J6XDb9yQAIAyon2SPTR6ro-PbZKyuhYj9D0ZPvRTzpygrKBDSJLyT91vr6GAY9Y1jf-h2F0dG5ldHOIAAAAAAAAAACEZXRoMpATfFTTAAAgCf__________gmlkgnY0gmlwhCKlH5aJc2VjcDI1NmsxoQLCbLYIjb6CHUVK6ZPDbmI63CN7RGpCtUs3ZgD7cHhTVoN1ZHCCD6E",
		"enr:-LG4QKziyglD2VXiSnk_RmqkyjLd4wLUARapHKQSMILmwS0Faq3x-V4NhMk8DQaj9wYvsd3XlQy-zubbse_tUKEcoSqGAY9Y1nghh2F0dG5ldHOIAAAAAAAAAACEZXRoMpATfFTTAAAgCf__________gmlkgnY0gmlwhCIjJtWJc2VjcDI1NmsxoQMYYxIU7-mZYJKnsxXjwMx7f5H9OUY_M9-7LLz-EtbRS4N1ZHCCD6E",
		"enr:-LG4QKIVzp4ZStj76D0xTWHSz7ENNHhK5MFXCJc7ouEvD6ssK7WQtZ9fvsC__kaT6Net3akp0GPHB6PyeK_R4C6QpJCGAY9Y1pDih2F0dG5ldHOIAAAAAAAAAACEZXRoMpATfFTTAAAgCf__________gmlkgnY0gmlwhCKOaNqJc2VjcDI1NmsxoQNaEX3qzrCEtLabdJ3CKxbQ-zRltgWcgw-2W23a9ohVa4N1ZHCCD6E",
		"enr:-LG4QDARu7otvyezvr_kV_lQ1m10AiGe2mMlCrzunB4ims60GtGbMzAqZ6S64rEGyqMLnQFF66u3UCszjq79kjZ4vz2GAY9Y1AiJh2F0dG5ldHOIAAAAAAAAAACEZXRoMpATfFTTAAAgCf__________gmlkgnY0gmlwhCKvNsmJc2VjcDI1NmsxoQJ5Z8clR7NXHxuty5jvOoJ3jH_SWDaJ9mrVutWGF2VT2YN1ZHCCD6E",
	}
	OverrideBeaconNetworkConfig(cfg)
}

// UseTestnet8Config sets the main beacon chain config for Testnet8.
func UseTestnet8Config() {
	beaconConfig = Testnet8Config()
}

// Testnet8Config defines the config for the Testnet8.
func Testnet8Config() *BeaconChainConfig {
	cfg := MainnetConfig().Copy()
	cfg.ConfigName = ConfigNames[Testnet8]
	cfg.DepositContractAddress = "0x6671Ed1732b6b5AF82724A1d1A94732D1AA37aa6"
	cfg.DepositChainID = gwatParams.Testnet8ChainConfig.ChainID.Uint64()
	cfg.DepositNetworkID = gwatParams.Testnet8ChainConfig.ChainID.Uint64()
	//cfg.DelegateForkSlot = types.Slot(gwatParams.Testnet8ChainConfig.ForkSlotDelegate)
	cfg.DelegateForkSlot = 2729920
	cfg.PrefixFinForkSlot = 4058240
	//todo require
	cfg.FinEth1ForkSlot = math.MaxUint64
	cfg.BlockVotingForkSlot = math.MaxUint64
	cfg.SlotsPerArchivedPoint = 2048

	cfg.SlotsPerEpoch = 32
	cfg.SecondsPerSlot = 4
	cfg.MinDepositAmount = 100 * 1e9
	cfg.MaxEffectiveBalance = 3200 * 1e9
	cfg.EjectionBalance = 1600 * 1e9
	cfg.EffectiveBalanceIncrement = 100 * 1e9
	cfg.OptValidatorsNum = 3_000_000

	cfg.InitializeForkSchedule()
	return cfg
}
