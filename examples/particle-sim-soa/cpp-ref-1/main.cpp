#include <iostream>
#include <vector>

struct Particle {
    double X;
    double Y;
    double Z;
    double VX;
    double VY;
    double VZ;
    double FX;
    double FY;
    double FZ;
    double Mass;
    int Tag;
    int ID;
};

Particle* NewParticle(int id, double x, double y, double z, double mass) {
    Particle* p = new Particle();
    p->X = x;
    p->Y = y;
    p->Z = z;
    p->VX = 0.0;
    p->VY = 0.0;
    p->VZ = 0.0;
    p->FX = 0.0;
    p->FY = 0.0;
    p->FZ = 0.0;
    p->Mass = mass;
    p->Tag = id % 4;
    p->ID = id;
    return p;
}

void ClearForces(std::vector<Particle*>& particles, int n) {
    for (int i = 0; i < n; i++) {
        particles[i]->FX = 0.0;
        particles[i]->FY = 0.0;
        particles[i]->FZ = 0.0;
    }
}

void ComputeForces(std::vector<Particle*>& particles, int n) {
    const double softening = 0.001;
    ClearForces(particles, n);

    for (int i = 0; i < n; i++) {
        Particle* pi = particles[i];
        for (int j = i + 1; j < n; j++) {
            Particle* pj = particles[j];

            double dx = pj->X - pi->X;
            double dy = pj->Y - pi->Y;
            double dz = pj->Z - pi->Z;
            double distSq = dx * dx + dy * dy + dz * dz + softening;

            double force = pi->Mass * pj->Mass / distSq;
            double fx = force * dx;
            double fy = force * dy;
            double fz = force * dz;

            pi->FX = pi->FX + fx;
            pi->FY = pi->FY + fy;
            pi->FZ = pi->FZ + fz;

            pj->FX = pj->FX - fx;
            pj->FY = pj->FY - fy;
            pj->FZ = pj->FZ - fz;
        }
    }
}

void Integrate(std::vector<Particle*>& particles, int n, double dt) {
    for (int i = 0; i < n; i++) {
        Particle* p = particles[i];
        double invMass = 1.0 / p->Mass;

        p->VX = p->VX + p->FX * invMass * dt;
        p->VY = p->VY + p->FY * invMass * dt;
        p->VZ = p->VZ + p->FZ * invMass * dt;

        p->X = p->X + p->VX * dt;
        p->Y = p->Y + p->VY * dt;
        p->Z = p->Z + p->VZ * dt;
    }
}

int ComputeKineticEnergy(std::vector<Particle*>& particles, int n) {
    double energy = 0.0;
    for (int i = 0; i < n; i++) {
        Particle* p = particles[i];
        double v2 = p->VX * p->VX + p->VY * p->VY + p->VZ * p->VZ;
        energy = energy + 0.5 * p->Mass * v2;
    }
    return static_cast<int>(energy);
}

int SumPositionX(std::vector<Particle*>& particles, int n) {
    double sum = 0.0;
    for (int i = 0; i < n; i++) {
        sum = sum + particles[i]->X;
    }
    return static_cast<int>(sum * 1000.0);
}

int CountTagGroup(std::vector<Particle*>& particles, int n, int targetTag) {
    int count = 0;
    for (int i = 0; i < n; i++) {
        if (particles[i]->Tag == targetTag) {
            count = count + 1;
        }
    }
    return count;
}

int main() {
    int numParticles = 5000;
    int numSteps = 50;
    double dt = 0.001;

    std::vector<Particle*> particles;
    particles.reserve(numParticles);

    for (int i = 0; i < numParticles; i++) {
        double fi = static_cast<double>(i);
        double fn = static_cast<double>(numParticles);

        double x = (fi / fn) * 10.0 - 5.0;
        double y = static_cast<double>((i * 7) % numParticles) / fn * 10.0 - 5.0;
        double z = static_cast<double>((i * 13) % numParticles) / fn * 10.0 - 5.0;
        double mass = 1.0 + static_cast<double>(i % 5) * 0.5;

        Particle* p = NewParticle(i, x, y, z, mass);
        particles.push_back(p);
    }

    std::cout << "Particles: " << numParticles << "\n";
    std::cout << "Steps: " << numSteps << "\n";

    int initialEnergy = ComputeKineticEnergy(particles, numParticles);
    int initialPosX = SumPositionX(particles, numParticles);

    for (int step = 0; step < numSteps; step++) {
        ComputeForces(particles, numParticles);
        Integrate(particles, numParticles, dt);
    }

    int finalEnergy = ComputeKineticEnergy(particles, numParticles);
    int finalPosX = SumPositionX(particles, numParticles);

    std::cout << "Initial energy: " << initialEnergy << "\n";
    std::cout << "Final energy: " << finalEnergy << "\n";
    std::cout << "Initial posX sum: " << initialPosX << "\n";
    std::cout << "Final posX sum: " << finalPosX << "\n";

    int c0 = CountTagGroup(particles, numParticles, 0);
    int c1 = CountTagGroup(particles, numParticles, 1);
    int c2 = CountTagGroup(particles, numParticles, 2);
    int c3 = CountTagGroup(particles, numParticles, 3);

    std::cout << "Tags: " << c0 << " " << c1 << " " << c2 << " " << c3 << "\n";

    if (initialEnergy == 0) {
        std::cout << "PASS: initial energy zero\n";
    } else {
        std::cout << "FAIL: initial energy should be zero\n";
    }

    if (finalEnergy > 0) {
        std::cout << "PASS: energy positive after simulation\n";
    } else {
        std::cout << "FAIL: energy should be positive\n";
    }

    if (finalPosX != initialPosX) {
        std::cout << "PASS: particles moved\n";
    } else {
        std::cout << "FAIL: particles should have moved\n";
    }

    if (c0 == 1250 && c1 == 1250 && c2 == 1250 && c3 == 1250) {
        std::cout << "PASS: tag distribution\n";
    } else {
        std::cout << "FAIL: unexpected tag distribution\n";
    }

    for (Particle* p : particles) {
        delete p;
    }

    return 0;
}