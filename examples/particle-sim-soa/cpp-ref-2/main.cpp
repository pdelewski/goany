#include <iostream>
#include <vector>

struct Particle {
    double x{};
    double y{};
    double z{};
    double vx{};
    double vy{};
    double vz{};
    double fx{};
    double fy{};
    double fz{};
    double mass{};
    int tag{};
    int id{};

    Particle(int id_, double x_, double y_, double z_, double mass_)
        : x(x_), y(y_), z(z_), mass(mass_), tag(id_ % 4), id(id_) {}
};

void clear_forces(std::vector<Particle>& particles) {
    for (auto& p : particles) {
        p.fx = 0.0;
        p.fy = 0.0;
        p.fz = 0.0;
    }
}

void compute_forces(std::vector<Particle>& particles) {
    constexpr double softening = 0.001;
    clear_forces(particles);

    const int n = static_cast<int>(particles.size());
    for (int i = 0; i < n; ++i) {
        auto& pi = particles[i];
        for (int j = i + 1; j < n; ++j) {
            auto& pj = particles[j];

            const double dx = pj.x - pi.x;
            const double dy = pj.y - pi.y;
            const double dz = pj.z - pi.z;
            const double dist_sq = dx * dx + dy * dy + dz * dz + softening;

            const double force = pi.mass * pj.mass / dist_sq;
            const double fx = force * dx;
            const double fy = force * dy;
            const double fz = force * dz;

            pi.fx += fx;
            pi.fy += fy;
            pi.fz += fz;

            pj.fx -= fx;
            pj.fy -= fy;
            pj.fz -= fz;
        }
    }
}

void integrate(std::vector<Particle>& particles, double dt) {
    for (auto& p : particles) {
        const double inv_mass = 1.0 / p.mass;

        p.vx += p.fx * inv_mass * dt;
        p.vy += p.fy * inv_mass * dt;
        p.vz += p.fz * inv_mass * dt;

        p.x += p.vx * dt;
        p.y += p.vy * dt;
        p.z += p.vz * dt;
    }
}

int compute_kinetic_energy(const std::vector<Particle>& particles) {
    double energy = 0.0;
    for (const auto& p : particles) {
        const double v2 = p.vx * p.vx + p.vy * p.vy + p.vz * p.vz;
        energy += 0.5 * p.mass * v2;
    }
    return static_cast<int>(energy);
}

int sum_position_x(const std::vector<Particle>& particles) {
    double sum = 0.0;
    for (const auto& p : particles) {
        sum += p.x;
    }
    return static_cast<int>(sum * 1000.0);
}

int count_tag_group(const std::vector<Particle>& particles, int target_tag) {
    int count = 0;
    for (const auto& p : particles) {
        if (p.tag == target_tag) {
            ++count;
        }
    }
    return count;
}

int main() {
    constexpr int num_particles = 5000;
    constexpr int num_steps = 50;
    constexpr double dt = 0.001;

    std::vector<Particle> particles;
    particles.reserve(num_particles);

    for (int i = 0; i < num_particles; ++i) {
        const double fi = static_cast<double>(i);
        const double fn = static_cast<double>(num_particles);

        const double x = (fi / fn) * 10.0 - 5.0;
        const double y = static_cast<double>((i * 7) % num_particles) / fn * 10.0 - 5.0;
        const double z = static_cast<double>((i * 13) % num_particles) / fn * 10.0 - 5.0;
        const double mass = 1.0 + static_cast<double>(i % 5) * 0.5;

        particles.emplace_back(i, x, y, z, mass);
    }

    std::cout << "Particles: " << num_particles << '\n';
    std::cout << "Steps: " << num_steps << '\n';

    const int initial_energy = compute_kinetic_energy(particles);
    const int initial_pos_x = sum_position_x(particles);

    for (int step = 0; step < num_steps; ++step) {
        compute_forces(particles);
        integrate(particles, dt);
    }

    const int final_energy = compute_kinetic_energy(particles);
    const int final_pos_x = sum_position_x(particles);

    std::cout << "Initial energy: " << initial_energy << '\n';
    std::cout << "Final energy: " << final_energy << '\n';
    std::cout << "Initial posX sum: " << initial_pos_x << '\n';
    std::cout << "Final posX sum: " << final_pos_x << '\n';

    const int c0 = count_tag_group(particles, 0);
    const int c1 = count_tag_group(particles, 1);
    const int c2 = count_tag_group(particles, 2);
    const int c3 = count_tag_group(particles, 3);

    std::cout << "Tags: " << c0 << ' ' << c1 << ' ' << c2 << ' ' << c3 << '\n';

    if (initial_energy == 0) {
        std::cout << "PASS: initial energy zero\n";
    } else {
        std::cout << "FAIL: initial energy should be zero\n";
    }

    if (final_energy > 0) {
        std::cout << "PASS: energy positive after simulation\n";
    } else {
        std::cout << "FAIL: energy should be positive\n";
    }

    if (final_pos_x != initial_pos_x) {
        std::cout << "PASS: particles moved\n";
    } else {
        std::cout << "FAIL: particles should have moved\n";
    }

    if (c0 == 1250 && c1 == 1250 && c2 == 1250 && c3 == 1250) {
        std::cout << "PASS: tag distribution\n";
    } else {
        std::cout << "FAIL: unexpected tag distribution\n";
    }

    return 0;
}