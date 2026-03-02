// Package calculators provides concrete Calculator implementations for the estimation engine.
//
// Each calculator estimates the time required for one specific phase of a VM migration
// (e.g. storage data transfer, post-migration troubleshooting). Calculators are designed
// to be composed via the estimation.Engine and accept input through estimation.Param slices.
package calculators
