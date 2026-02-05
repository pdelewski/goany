#ifndef GOANY_PANIC_HPP
#define GOANY_PANIC_HPP

#include <iostream>
#include <string>
#include <cstdlib>

void goany_panic(const std::string& msg) {
    std::cerr << "panic: " << msg << std::endl;
    exit(1);
}

#endif // GOANY_PANIC_HPP
