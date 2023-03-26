

#include <stdio.h>     
#include <stdlib.h>
#include <getopt.h>
#include <fcntl.h>
#include <unistd.h>

#include <vector>
#include <iostream>
#include <string>
#include <sstream>
#include <fstream>
#include <filesystem>
#include <thread>
#include <algorithm>
#include <mutex>

#include "deps/wyhash.h"
#include "deps/hash_sha256.h"

namespace fs = std::filesystem;

typedef struct {
    bool isStrict;
    bool isColon;
    bool isSha256;
    int concurrentHandles;
} Args;

typedef struct {
    std::string path;
    std::vector<uint8_t> hash;
} Task;

std::mutex read_mutex;

bool parseArgs(int argc, char *argv[], Args &args) {

    static struct option long_options[] = {
        {"strict",             no_argument,             0,  1 },
        {"colon",              no_argument,             0,  2 },
        {"sha256",             no_argument,             0,  3 },
        {"concurrent-handles", required_argument,       0,  4 },
        {0,                    0,                       0,  0 }
    };

    int ret = 0;
    int option_index = 0;
    while ((ret = getopt_long(argc, argv, "", long_options, &option_index)) != -1) {
        switch (ret) {
            case 1:
                args.isStrict = true;
                break;
            case 2:
                args.isColon = true;
                break;
            case 3:
                args.isSha256 = true;
                break;
            case 4:
                args.concurrentHandles = atoi(optarg);
                break;
            default:
                printf("Usage: check [--strict] [--colon] [--sha256] [--concurrnt-handles=<n>]\n");
                return false;
        }
    }

    return true;
}

std::vector<uint8_t> getHash(char* payload, size_t size, Args args) {
    
    if (args.isSha256) {
        if (size == 0){
            return std::vector<uint8_t>(32, 0);
        }

        hash_sha256 hash;
        hash.sha256_init();
        hash.sha256_update((uint8_t*) &payload[0], size);
        auto result = hash.sha256_final();
        std::vector<uint8_t> dest(std::begin(result), std::end(result));
        return dest;
    }
    else {
        if (size == 0) {
            return std::vector<uint8_t>(8, 0);
        }

        std::vector<uint8_t> hash;
        auto secret = 0x1234567899887766UL;
        auto result = wyhash((void *)&payload[0], size, 0x1UL, &secret);
        hash.push_back((uint8_t) result);
        hash.push_back((uint8_t) (result >> 8));
        hash.push_back((uint8_t) (result >> 16));
        hash.push_back((uint8_t) (result >> 24));
        hash.push_back((uint8_t) (result >> 32));
        hash.push_back((uint8_t) (result >> 40));
        hash.push_back((uint8_t) (result >> 48));
        hash.push_back((uint8_t) (result >> 56));
        return hash;
    }
}

std::string printHex(std::vector<uint8_t>& hash, Args args) {
    std::stringstream ss;
    
    for (int i=0; i<hash.size(); i++) {
        char hex[3];
        sprintf(hex,"%02X", hash[i]);
        ss << hex;
        if (args.isColon && i < hash.size() - 1) {
            ss << ":";
        }
    }

    return ss.str();
}

void worker(std::vector<Task>& tasks, int start, int end, Args args) {
    int conc = args.concurrentHandles;

    char* buffer = new char[1024*1024];

    size_t prev_size = 1024*1024;


    for (int i=start; i<end; i++) {
        auto &t = tasks[i]; 
        try {
            auto canonical_path = fs::canonical(t.path);
            size_t size = fs::file_size(canonical_path);

            if(size > prev_size) {
                prev_size = size;
                delete buffer;
                buffer = new char[size];
            }

            int fd = open(canonical_path.c_str(), O_CLOEXEC, S_IRUSR);
            if (fd == -1){
                std::cerr << "Error opening file: " << canonical_path.c_str() << std::endl;
                if (args.isStrict){
                    exit(1);
                }
                continue;
            }

            bool finished = false;
            size_t offset = 0;

            while(!finished) {
                finished = true;
                int n = read(fd, &buffer[offset], size - offset + 1 );
                
                if (n == -1) {
                    std::cerr << "Error reading file: " << t.path.c_str() << " " << errno << std::endl;
                    if (args.isStrict){
                        exit(1);
                    }
                    break;
                }
                offset += n;

                if(offset != size) {
                    finished = false;
                }
                else {
                    t.hash = getHash(buffer, size, args);
                }
            }

            if (fd != -1){
                close(fd);
            }
        }
        catch(const std::exception& ex) {
            std::cerr << t.path << ": " << ex.what() << std::endl;
            if (args.isStrict) {
                exit(1);
            }
        }   
    }

}


int main(int argc, char *argv[]) {

    Args args;
    args.isColon = false;
    args.isStrict = false;
    args.isSha256 = false;
    args.concurrentHandles = 10;

    if(!parseArgs(argc, argv, args)) {
        exit(1);
    }

    std::vector<Task> tasks;

    std::string line;
    while (std::getline(std::cin, line))
    {
        Task task;
        task.path = line;
        tasks.push_back(task);
    }

    sort(tasks.begin(), tasks.end(), [](auto l, auto r) { return l.path < r.path; });

    const auto processor_count = std::thread::hardware_concurrency();
    int chunk = tasks.size() / processor_count;

    std::vector<std::thread> workers;
    for(int start=0,end=0; end < tasks.size(); ) {
        end = std::min(start+chunk, (int)tasks.size());
        workers.push_back(std::thread(worker, std::ref(tasks), start, end, args));
        start = end;
    }

    for (auto &w : workers) {
        w.join();
    }

    for (auto t : tasks) {
        std::cout << printHex(t.hash, args) << " " << t.path << std::endl;
    }

    exit(EXIT_SUCCESS);
}