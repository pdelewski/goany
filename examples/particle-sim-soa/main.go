package main

import "fmt"

// SoA layout: each field is a separate contiguous array.
// Force loop reads X[], Y[], Z[], Mass[] — all contiguous, 100% cache utilization.
// Compare with AoS (particle-sim) where 67% of cache is wasted on cold fields.

// Particles stores all particle data in Structure-of-Arrays layout.
type Particles struct {
	X    []float64
	Y    []float64
	Z    []float64
	VX   []float64
	VY   []float64
	VZ   []float64
	FX   []float64
	FY   []float64
	FZ   []float64
	Mass []float64
	Tag  []int
	ID   []int
}

func NewParticles(capacity int) *Particles {
	p := &Particles{}
	p.X = make([]float64, 0)
	p.Y = make([]float64, 0)
	p.Z = make([]float64, 0)
	p.VX = make([]float64, 0)
	p.VY = make([]float64, 0)
	p.VZ = make([]float64, 0)
	p.FX = make([]float64, 0)
	p.FY = make([]float64, 0)
	p.FZ = make([]float64, 0)
	p.Mass = make([]float64, 0)
	p.Tag = make([]int, 0)
	p.ID = make([]int, 0)
	return p
}

func AddParticle(ps *Particles, id int, x float64, y float64, z float64, mass float64) {
	ps.X = append(ps.X, x)
	ps.Y = append(ps.Y, y)
	ps.Z = append(ps.Z, z)
	ps.VX = append(ps.VX, 0.0)
	ps.VY = append(ps.VY, 0.0)
	ps.VZ = append(ps.VZ, 0.0)
	ps.FX = append(ps.FX, 0.0)
	ps.FY = append(ps.FY, 0.0)
	ps.FZ = append(ps.FZ, 0.0)
	ps.Mass = append(ps.Mass, mass)
	ps.Tag = append(ps.Tag, id%4)
	ps.ID = append(ps.ID, id)
}

// ClearForces zeroes all force accumulators.
func ClearForces(ps *Particles, n int) {
	for i := 0; i < n; i++ {
		ps.FX[i] = 0.0
		ps.FY[i] = 0.0
		ps.FZ[i] = 0.0
	}
}

// ComputeForces computes all-pairs gravitational forces (O(n^2)).
// Hot inner loop reads X[], Y[], Z[], Mass[] — all contiguous arrays.
// Each cache line holds 8 float64 values from the SAME field.
func ComputeForces(ps *Particles, n int) {
	softening := 0.001
	ClearForces(ps, n)
	for i := 0; i < n; i++ {
		pix := ps.X[i]
		piy := ps.Y[i]
		piz := ps.Z[i]
		pim := ps.Mass[i]
		fix := 0.0
		fiy := 0.0
		fiz := 0.0
		for j := i + 1; j < n; j++ {
			dx := ps.X[j] - pix
			dy := ps.Y[j] - piy
			dz := ps.Z[j] - piz
			distSq := dx*dx + dy*dy + dz*dz + softening
			force := pim * ps.Mass[j] / distSq
			fx := force * dx
			fy := force * dy
			fz := force * dz
			fix = fix + fx
			fiy = fiy + fy
			fiz = fiz + fz
			ps.FX[j] = ps.FX[j] - fx
			ps.FY[j] = ps.FY[j] - fy
			ps.FZ[j] = ps.FZ[j] - fz
		}
		ps.FX[i] = ps.FX[i] + fix
		ps.FY[i] = ps.FY[i] + fiy
		ps.FZ[i] = ps.FZ[i] + fiz
	}
}

// Integrate updates velocities and positions using Euler method.
func Integrate(ps *Particles, n int, dt float64) {
	for i := 0; i < n; i++ {
		invMass := 1.0 / ps.Mass[i]
		ps.VX[i] = ps.VX[i] + ps.FX[i]*invMass*dt
		ps.VY[i] = ps.VY[i] + ps.FY[i]*invMass*dt
		ps.VZ[i] = ps.VZ[i] + ps.FZ[i]*invMass*dt
		ps.X[i] = ps.X[i] + ps.VX[i]*dt
		ps.Y[i] = ps.Y[i] + ps.VY[i]*dt
		ps.Z[i] = ps.Z[i] + ps.VZ[i]*dt
	}
}

// ComputeKineticEnergy returns total kinetic energy as integer (truncated).
func ComputeKineticEnergy(ps *Particles, n int) int {
	energy := 0.0
	for i := 0; i < n; i++ {
		v2 := ps.VX[i]*ps.VX[i] + ps.VY[i]*ps.VY[i] + ps.VZ[i]*ps.VZ[i]
		energy = energy + 0.5*ps.Mass[i]*v2
	}
	return int(energy)
}

// SumPositionX returns sum of all X positions scaled to integer.
func SumPositionX(ps *Particles, n int) int {
	sum := 0.0
	for i := 0; i < n; i++ {
		sum = sum + ps.X[i]
	}
	return int(sum * 1000.0)
}

// CountTagGroup counts particles matching the given tag value.
func CountTagGroup(ps *Particles, n int, targetTag int) int {
	count := 0
	for i := 0; i < n; i++ {
		if ps.Tag[i] == targetTag {
			count = count + 1
		}
	}
	return count
}

func main() {
	numParticles := 5000
	numSteps := 50
	dt := 0.001

	// Create particles in deterministic spatial pattern (same as AoS version)
	ps := NewParticles(numParticles)
	for i := 0; i < numParticles; i++ {
		fi := float64(i)
		fn := float64(numParticles)
		x := (fi / fn) * 10.0 - 5.0
		y := float64((i*7)%numParticles) / fn*10.0 - 5.0
		z := float64((i*13)%numParticles) / fn*10.0 - 5.0
		mass := 1.0 + float64(i%5)*0.5
		AddParticle(ps, i, x, y, z, mass)
	}

	fmt.Printf("Particles: %d\n", numParticles)
	fmt.Printf("Steps: %d\n", numSteps)

	// Record initial state
	initialEnergy := ComputeKineticEnergy(ps, numParticles)
	initialPosX := SumPositionX(ps, numParticles)

	// Run N-body simulation
	for step := 0; step < numSteps; step++ {
		ComputeForces(ps, numParticles)
		Integrate(ps, numParticles, dt)
	}

	// Record final state
	finalEnergy := ComputeKineticEnergy(ps, numParticles)
	finalPosX := SumPositionX(ps, numParticles)

	fmt.Printf("Initial energy: %d\n", initialEnergy)
	fmt.Printf("Final energy: %d\n", finalEnergy)
	fmt.Printf("Initial posX sum: %d\n", initialPosX)
	fmt.Printf("Final posX sum: %d\n", finalPosX)

	// Count by tag (accesses cold field — not used in hot loops)
	c0 := CountTagGroup(ps, numParticles, 0)
	c1 := CountTagGroup(ps, numParticles, 1)
	c2 := CountTagGroup(ps, numParticles, 2)
	c3 := CountTagGroup(ps, numParticles, 3)
	fmt.Printf("Tags: %d %d %d %d\n", c0, c1, c2, c3)

	// Verification
	if initialEnergy == 0 {
		fmt.Println("PASS: initial energy zero")
	} else {
		fmt.Println("FAIL: initial energy should be zero")
	}

	if finalEnergy > 0 {
		fmt.Println("PASS: energy positive after simulation")
	} else {
		fmt.Println("FAIL: energy should be positive")
	}

	if finalPosX != initialPosX {
		fmt.Println("PASS: particles moved")
	} else {
		fmt.Println("FAIL: particles should have moved")
	}

	if c0 == 1250 && c1 == 1250 && c2 == 1250 && c3 == 1250 {
		fmt.Println("PASS: tag distribution")
	} else {
		fmt.Println("FAIL: unexpected tag distribution")
	}
}
