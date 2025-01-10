/*
The folder contains the Validator client service.
The Validator client service runs in separate process and implements workflow of validators
in accordance withe assined roles:
- attestation
- aggregation of attestations
- pre voting
- propose block
- sync-committee
- aggregation of  sync-committee

The Validator client service interact with cooddinator node over validator api.
*/
package validator
