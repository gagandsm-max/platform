import http.server
import json
import logging
import os
import signal
import sys
import threading
import time

class JsonFormatter(logging.Formatter):
    def format(self, record):
        log_obj = {
            "timestamp": self.formatTime(record, self.datefmt),
            "level": record.levelname,
            "service": "platform-analytics",
            "message": record.getMessage(),
            "logger": record.name,
            "environment": os.getenv("PLATFORM_ENV", "development"),
        }
        if hasattr(record, "correlation_id"):
            log_obj["correlation_id"] = record.correlation_id
        return json.dumps(log_obj)

handler = logging.StreamHandler(sys.stdout)
handler.setFormatter(JsonFormatter())
logger = logging.getLogger("platform.analytics")
logger.setLevel(os.getenv("PLATFORM_LOG_LEVEL", "INFO"))
logger.addHandler(handler)
logger.propagate = False

class HealthHandler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path in ("/health", "/healthz"):
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            resp = {
                "status": "UP",
                "service": "platform-analytics",
                "version": os.getenv("PLATFORM_VERSION", "0.1.0-dev"),
            }
            self.wfile.write(json.dumps(resp).encode("utf-8"))
        else:
            self.send_response(404)
            self.end_headers()

    def log_message(self, format, *args):
        # Suppress standard logging to stdout to keep JSON clean
        pass

def start_health_server(port):
    server = http.server.HTTPServer(("0.0.0.0", port), HealthHandler)
    server_thread = threading.Thread(target=server.serve_forever, daemon=True)
    server_thread.start()
    return server

def main():
    logger.info("Platform Analytics engine initializing")
    health_port = int(os.getenv("ANALYTICS_HEALTH_PORT", "8090"))
    server = start_health_server(health_port)
    logger.info(f"Health server listening on port {health_port}")

    running = True

    def signal_handler(signum, frame):
        nonlocal running
        logger.info(f"Received signal {signum}, initiating graceful shutdown")
        running = False

    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)

    logger.info("Platform Analytics worker loop started")
    while running:
        try:
            # Synthetic background computation cycle
            time.sleep(5)
        except Exception as e:
            logger.error(f"Error in analytics worker loop: {e}")

    server.shutdown()
    logger.info("Platform Analytics engine shutdown cleanly")

if __name__ == "__main__":
    main()
