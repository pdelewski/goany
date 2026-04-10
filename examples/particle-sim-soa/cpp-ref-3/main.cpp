#include <iostream>
#include <vector>

struct Particles {
    std::vector<double> x, y, z;
    std::vector<double> vx, vy, vz;
    std::vector<double> fx, fy, fz;
    std::vector<double> mass;
    std::vector<int> tag, id;

    explicit Particles(int n)
        : x(n), y(n), z(n),
          vx(n, 0.0), vy(n, 0.0), vz(n, 0.0),
          fx(n, 0.0), fy(n, 0.0), fz(n, 0.0),
          mass(n),
          tag(n), id(n) {}

    [[nodiscard]] int size() const {
        return static_cast<int>(x.size());
    }
};

void init_particle(Particles& p, int i, double px, double py, double pz, double m) {
    p.x[i] = px;
    p.y[i] = py;
    p.z[i] = pz;

    p.vx[i] = 0.0;
    p.vy[i] = 0.0;
    p.vz[i] = 0.0;

    p.fx[i] = 0.0;
    p.fy[i] = 0.0;
    p.fz[i] = 0.0;

    p.mass[i] = m;
    p.tag[i] = i % 4;
    p.id[i] = i;
}

void clear_forces(Particles& p) {
    const int n = p.size();
    double* fx = p.fx.data();
    double* fy = p.fy.data();
    double* fz = p.fz.data();

    for (int i = 0; i < n; ++i) {
        fx[i] = 0.0;
        fy[i] = 0.0;
        fz[i] = 0.0;
    }
}

void compute_forces(Particles& p) {
    constexpr double softening = 0.001;
    const int n = p.size();

    double* x = p.x.data();
    double* y = p.y.data();
    double* z = p.z.data();
    double* fx = p.fx.data();
    double* fy = p.fy.data();
    double* fz = p.fz.data();
    double* mass = p.mass.data();

    for (int i = 0; i < n; ++i) {
        fx[i] = 0.0;
        fy[i] = 0.0;
        fz[i] = 0.0;
    }

    for (int i = 0; i < n; ++i) {
        const double xi = x[i];
        const double yi = y[i];
        const double zi = z[i];
        const double mi = mass[i];

        double fxi = 0.0;
        double fyi = 0.0;
        double fzi = 0.0;

        for (int j = i + 1; j < n; ++j) {
            const double dx = x[j] - xi;
            const double dy = y[j] - yi;
            const double dz = z[j] - zi;
            const double dist_sq = dx * dx + dy * dy + dz * dz + softening;

            const double force = mi * mass[j] / dist_sq;
            const double dfx = force * dx;
            const double dfy = force * dy;
            const double dfz = force * dz;

            fxi += dfx;
            fyi += dfy;
            fzi += dfz;

            fx[j] -= dfx;
            fy[j] -= dfy;
            fz[j] -= dfz;
        }

        fx[i] += fxi;
        fy[i] += fyi;
        fz[i] += fzi;
    }
}

void integrate(Particles& p, double dt) {
    const int n = p.size();

    double* x = p.x.data();
    double* y = p.y.data();
    double* z = p.z.data();
    double* vx = p.vx.data();
    double* vy = p.vy.data();
    double* vz = p.vz.data();
    double* fx = p.fx.data();
    double* fy = p.fy.data();
    double* fz = p.fz.data();
    double* mass = p.mass.data();

    for (int i = 0; i < n; ++i) {
        const double inv_mass = 1.0 / mass[i];

        vx[i] += fx[i] * inv_mass * dt;
        vy[i] += fy[i] * inv_mass * dt;
        vz[i] += fz[i] * inv_mass * dt;

        x[i] += vx[i] * dt;
        y[i] += vy[i] * dt;
        z[i] += vz[i] * dt;
    }
}

int compute_kinetic_energy(const Particles& p) {
    const int n = p.size();

    const double* vx = p.vx.data();
    const double* vy = p.vy.data();
    const double* vz = p.vz.data();
    const double* mass = p.mass.data();

    double energy = 0.0;
    for (int i = 0; i < n; ++i) {
        const double v2 = vx[i] * vx[i] + vy[i] * vy[i] + vz[i] * vz[i];
        energy += 0.5 * mass[i] * v2;
    }

    return static_cast<int>(energy);
}

int sum_position_x(const Particles& p) {
    const int n = p.size();
    const double* x = p.x.data();

    double sum = 0.0;
    for (int i = 0; i < n; ++i) {
        sum += x[i];
    }

    return static_cast<int>(sum * 1000.0);
}

int count_tag_group(const Particles& p, int target_tag) {
    const int n = p.size();
    const int* tag = p.tag.data();

    int count = 0;
    for (int i = 0; i < n; ++i) {
        if (tag[i] == target_tag) {
            ++count;
        }
    }
    return count;
}

int main() {
    constexpr int num_particles = 5000;
    constexpr int num_steps = 50;
    constexpr double dt = 0.001;

    Particles particles(num_particles);

    for (int i = 0; i < num_particles; ++i) {
        const double fi = static_cast<double>(i);
        const double fn = static_cast<double>(num_particles);

        const double x = (fi / fn) * 10.0 - 5.0;
        const double y = static_cast<double>((i * 7) % num_particles) / fn * 10.0 - 5.0;
        const double z = static_cast<double>((i * 13) % num_particles) / fn * 10.0 - 5.0;
        const double mass = 1.0 + static_cast<double>(i % 5) * 0.5;

        init_particle(particles, i, x, y, z, mass);
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