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

// UseTestConfig sets the main beacon chain config for tests.
func UseTestConfig() {
	beaconConfig = TestConfig()
}

// TestConfig defines the config for the tests.
func TestConfig() *BeaconChainConfig {
	cfg := MainnetConfig().Copy()
	cfg.ConfigName = ConfigNames[Testnet8]
	cfg.PresetBase = "mainnet"

	cfg.MinDepositAmount = 100 * 1e9
	cfg.MaxEffectiveBalance = 3200 * 1e9
	cfg.EjectionBalance = 1600 * 1e9
	cfg.EffectiveBalanceIncrement = 100 * 1e9
	cfg.OptValidatorsNum = 3_000_000

	cfg.SecondsPerSlot = 4

	cfg.InitializeForkSchedule()

	return cfg.Copy()
}
