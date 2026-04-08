package main

import "fmt"

// Particle represents a body in an N-body simulation.
//
// Field layout for SoA analysis:
//   Hot  (force calc inner loop): X, Y, Z, Mass — 4 of 12 fields
//   Warm (integration step):      VX, VY, VZ, FX, FY, FZ
//   Cold (reporting only):        Tag, ID
//
// AoS: each particle = 96 bytes (1.5 cache lines). Force loop loads all 96 bytes
//       but only reads 32 bytes (X,Y,Z,Mass) → 33% cache utilization.
// SoA: X[], Y[], Z[], Mass[] contiguous → 100% cache utilization in force loop.
type Particle struct {
	X    float64
	Y    float64
	Z    float64
	VX   float64
	VY   float64
	VZ   float64
	FX   float64
	FY   float64
	FZ   float64
	Mass float64
	Tag  int
	ID   int
}

func NewParticle(id int, x float64, y float64, z float64, mass float64) *Particle {
	p := &Particle{}
	p.X = x
	p.Y = y
	p.Z = z
	p.VX = 0.0
	p.VY = 0.0
	p.VZ = 0.0
	p.FX = 0.0
	p.FY = 0.0
	p.FZ = 0.0
	p.Mass = mass
	p.Tag = id % 4
	p.ID = id
	return p
}

// ClearForces zeroes all force accumulators.
func ClearForces(particles []*Particle, n int) {
	for i := 0; i < n; i++ {
		particles[i].FX = 0.0
		particles[i].FY = 0.0
		particles[i].FZ = 0.0
	}
}

// ComputeForces computes all-pairs gravitational forces (O(n^2)).
// Hot inner loop reads only X, Y, Z, Mass per particle pair.
// This is the primary SoA benchmark target — with AoS, 67% of each
// cache line is wasted on VX,VY,VZ,FX,FY,FZ,Tag,ID.
func ComputeForces(particles []*Particle, n int) {
	softening := 0.001
	ClearForces(particles, n)
	for i := 0; i < n; i++ {
		pi := particles[i]
		for j := i + 1; j < n; j++ {
			pj := particles[j]
			dx := pj.X - pi.X
			dy := pj.Y - pi.Y
			dz := pj.Z - pi.Z
			distSq := dx*dx + dy*dy + dz*dz + softening
			// Softened gravity: F = m1*m2 / (r^2 + eps), no sqrt needed
			force := pi.Mass * pj.Mass / distSq
			fx := force * dx
			fy := force * dy
			fz := force * dz
			pi.FX = pi.FX + fx
			pi.FY = pi.FY + fy
			pi.FZ = pi.FZ + fz
			pj.FX = pj.FX - fx
			pj.FY = pj.FY - fy
			pj.FZ = pj.FZ - fz
		}
	}
}

// Integrate updates velocities and positions using Euler method.
func Integrate(particles []*Particle, n int, dt float64) {
	for i := 0; i < n; i++ {
		p := particles[i]
		invMass := 1.0 / p.Mass
		p.VX = p.VX + p.FX*invMass*dt
		p.VY = p.VY + p.FY*invMass*dt
		p.VZ = p.VZ + p.FZ*invMass*dt
		p.X = p.X + p.VX*dt
		p.Y = p.Y + p.VY*dt
		p.Z = p.Z + p.VZ*dt
	}
}

// ComputeKineticEnergy returns total kinetic energy as integer (truncated).
// No multiplier — value must fit in 32-bit int for all backends.
func ComputeKineticEnergy(particles []*Particle, n int) int {
	energy := 0.0
	for i := 0; i < n; i++ {
		p := particles[i]
		v2 := p.VX*p.VX + p.VY*p.VY + p.VZ*p.VZ
		energy = energy + 0.5*p.Mass*v2
	}
	return int(energy)
}

// SumPositionX returns sum of all X positions scaled to integer.
func SumPositionX(particles []*Particle, n int) int {
	sum := 0.0
	for i := 0; i < n; i++ {
		sum = sum + particles[i].X
	}
	return int(sum * 1000.0)
}

// CountTagGroup counts particles matching the given tag value.
func CountTagGroup(particles []*Particle, n int, targetTag int) int {
	count := 0
	for i := 0; i < n; i++ {
		if particles[i].Tag == targetTag {
			count = count + 1
		}
	}
	return count
}

func main() {
	numParticles := 5000
	numSteps := 50
	dt := 0.001

	// Create particles in deterministic spatial pattern
	particles := make([]*Particle, 0)
	for i := 0; i < numParticles; i++ {
		fi := float64(i)
		fn := float64(numParticles)
		x := (fi / fn) * 10.0 - 5.0
		y := float64((i*7)%numParticles) / fn*10.0 - 5.0
		z := float64((i*13)%numParticles) / fn*10.0 - 5.0
		mass := 1.0 + float64(i%5)*0.5
		p := NewParticle(i, x, y, z, mass)
		particles = append(particles, p)
	}

	fmt.Printf("Particles: %d\n", numParticles)
	fmt.Printf("Steps: %d\n", numSteps)

	// Record initial state
	initialEnergy := ComputeKineticEnergy(particles, numParticles)
	initialPosX := SumPositionX(particles, numParticles)

	// Run N-body simulation
	for step := 0; step < numSteps; step++ {
		ComputeForces(particles, numParticles)
		Integrate(particles, numParticles, dt)
	}

	// Record final state
	finalEnergy := ComputeKineticEnergy(particles, numParticles)
	finalPosX := SumPositionX(particles, numParticles)

	fmt.Printf("Initial energy: %d\n", initialEnergy)
	fmt.Printf("Final energy: %d\n", finalEnergy)
	fmt.Printf("Initial posX sum: %d\n", initialPosX)
	fmt.Printf("Final posX sum: %d\n", finalPosX)

	// Count by tag (accesses cold field — not used in hot loops)
	c0 := CountTagGroup(particles, numParticles, 0)
	c1 := CountTagGroup(particles, numParticles, 1)
	c2 := CountTagGroup(particles, numParticles, 2)
	c3 := CountTagGroup(particles, numParticles, 3)
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
