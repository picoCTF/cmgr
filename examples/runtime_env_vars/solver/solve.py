#!/usr/bin/env python3

import os
import argparse
import urllib.request
import sys

def solve():
    parser = argparse.ArgumentParser(description="solve script for runtime_env_vars")
    parser.add_argument("--host", default="challenge", help="the host for the instance")
    parser.add_argument("--port", type=int, default=8000, help="the port of the instance")
    args = parser.parse_args()

    print(f"Testing runtime environment variables at http://{args.host}:{args.port}...")
    
    url = f"http://{args.host}:{args.port}/"
    try:
        response = urllib.request.urlopen(url).read().decode('utf-8')
    except Exception as e:
        print(f"Failed to connect to server: {e}")
        sys.exit(1)
        
    lines = response.strip().split('\n')
    data = {}
    for line in lines:
        if '=' in line:
            k, v = line.split('=', 1)
            data[k] = v

    print(f"Server responded with environment info")
    
    flag = data.get('FLAG')
    if flag and flag != 'Unknown':
        with open("flag", "w") as f:
            f.write(flag)
        print(f"Success! Flag retrieved: {flag}")
    else:
        print("Error: flag was missing or unknown.")
        sys.exit(1)
        
if __name__ == "__main__":
    solve()
