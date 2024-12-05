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

// UseTestnet5NetworkConfig uses the Testnet5 specific network config.
func UseTestnet5NetworkConfig() {
	cfg := BeaconNetworkConfig().Copy()
	cfg.BootstrapNodes = []string{}
	OverrideBeaconNetworkConfig(cfg)
}

// UseTestnet5Config sets the main beacon chain config for Testnet5.
func UseTestnet5Config() {
	beaconConfig = Testnet5Config()
}

// Testnet5Config defines the config for the Testnet5.
func Testnet5Config() *BeaconChainConfig {
	cfg := MainnetConfig().Copy()

	cfg.ConfigName = ConfigNames[Testnet5]
	//cfg.DepositContractAddress = "0x501bf68fC5945FF6449A18d301C99016fEBe2437"
	cfg.DepositChainID = 1501865
	cfg.DepositNetworkID = 1501865

	cfg.DelegateForkSlot = 0
	cfg.PrefixFinForkSlot = 0
	cfg.FinEth1ForkSlot = 0
	cfg.BlockVotingForkSlot = 0
	//cfg.SlotsPerArchivedPoint = 2048

	//cfg.SlotsPerEpoch = 32
	//cfg.SecondsPerSlot = 6

	//cfg.MinDepositAmount = 1000 * 1e9
	//cfg.MaxEffectiveBalance = 32000 * 1e9
	//cfg.EjectionBalance = 16000 * 1e9
	//cfg.EffectiveBalanceIncrement = 1000 * 1e9
	//cfg.OptValidatorsNum = 300_000

	cfg.InitializeForkSchedule()
	return cfg
}
