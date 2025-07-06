#!/bin/bash

echo "KeyBench Examples"
echo "================="
echo

echo "1. Basic RSA benchmark (default settings):"
echo "   ./keybench"
echo

echo "2. Benchmark all algorithms with common key sizes:"
echo "   ./keybench -a rsa,ecdsa,ed25519 -k 2048,256,256 -i 10"
echo

echo "3. High-performance RSA test with parallel workers:"
echo "   ./keybench -a rsa -k 4096 -i 100 -p 4 -v"
echo

echo "4. Quick ECDSA comparison:"
echo "   ./keybench -a ecdsa -k 224,256,384,521 -i 20"
echo

echo "5. Export detailed results to JSON:"
echo "   ./keybench -a rsa -k 1024,2048,4096 -f json -o results.json"
echo

echo "6. CSV export for spreadsheet analysis:"
echo "   ./keybench -a rsa,ecdsa -k 2048,256 -i 50 -f csv -o benchmark.csv"
echo

echo "7. Stress test with large RSA keys:"
echo "   ./keybench -a rsa -k 8192,16384 -i 5 -t 600"
echo

echo "8. Ed25519 performance test:"
echo "   ./keybench -a ed25519 -i 1000 -p 8"
echo