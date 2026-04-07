#!/usr/bin/env python3

import os
from http.server import BaseHTTPRequestHandler, HTTPServer

class SimpleHTTPRequestHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header('Content-Type', 'text/plain')
        self.end_headers()
        
        user_id = os.environ.get('CMGR_USER_ID', 'Not Set')
        custom_var = os.environ.get('CMGR_CUSTOM_VAR', 'Not Set')
        
        flag = "Unknown"
        try:
            with open('/challenge/flag.txt', 'r') as f:
                flag = f.read().strip()
        except Exception:
            pass
        
        response = f"CMGR_USER_ID={user_id}\nCMGR_CUSTOM_VAR={custom_var}\nFLAG={flag}\n"
        self.wfile.write(response.encode('utf-8'))

if __name__ == '__main__':
    port = 8000
    print(f"Starting server on port {port}...")
    httpd = HTTPServer(('0.0.0.0', port), SimpleHTTPRequestHandler)
    httpd.serve_forever()
