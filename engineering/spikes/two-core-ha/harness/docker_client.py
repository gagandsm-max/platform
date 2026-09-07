import http.client
import json
import socket
import time
import urllib.parse
from typing import Any, Dict, List, Optional


class UnixSocketHTTPConnection(http.client.HTTPConnection):
    def __init__(self, socket_path: str = "/var/run/docker.sock", timeout: int = 30):
        super().__init__("localhost", timeout=timeout)
        self.socket_path = socket_path

    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(self.timeout)
        self.sock.connect(self.socket_path)


class DockerController:
    """Interacts with local Docker daemon over /var/run/docker.sock."""

    def __init__(self, socket_path: str = "/var/run/docker.sock"):
        self.socket_path = socket_path

    def _request(self, method: str, path: str, body: Optional[Dict[str, Any]] = None) -> tuple[int, Any]:
        conn = UnixSocketHTTPConnection(self.socket_path)
        headers = {"Host": "localhost"}
        payload = None
        if body is not None:
            headers["Content-Type"] = "application/json"
            payload = json.dumps(body)
        conn.request(method, path, body=payload, headers=headers)
        resp = conn.getresponse()
        resp_data = resp.read()
        conn.close()
        try:
            data = json.loads(resp_data.decode("utf-8")) if resp_data else None
        except Exception:
            data = resp_data.decode("utf-8", errors="ignore")
        return resp.status, data

    def inspect_container(self, name: str) -> Optional[Dict[str, Any]]:
        status, data = self._request("GET", f"/containers/{urllib.parse.quote(name)}/json")
        if status == 200:
            return data
        return None

    def is_running(self, name: str) -> bool:
        info = self.inspect_container(name)
        if info and "State" in info:
            return info["State"].get("Running", False)
        return False

    def is_healthy(self, name: str) -> bool:
        info = self.inspect_container(name)
        if info and "State" in info:
            health = info["State"].get("Health", {})
            if health:
                return health.get("Status") == "healthy"
            return info["State"].get("Running", False)
        return False

    def stop_container(self, name: str, timeout_sec: int = 5) -> bool:
        status, _ = self._request("POST", f"/containers/{urllib.parse.quote(name)}/stop?t={timeout_sec}")
        return status in (204, 304)

    def kill_container(self, name: str, signal: str = "SIGKILL") -> bool:
        status, _ = self._request("POST", f"/containers/{urllib.parse.quote(name)}/kill?signal={signal}")
        return status in (204, 304)

    def start_container(self, name: str) -> bool:
        status, _ = self._request("POST", f"/containers/{urllib.parse.quote(name)}/start")
        return status in (204, 304)

    def restart_container(self, name: str, timeout_sec: int = 5) -> bool:
        status, _ = self._request("POST", f"/containers/{urllib.parse.quote(name)}/restart?t={timeout_sec}")
        return status in (204, 304)

    def disconnect_network(self, container: str, network: str = "ha_spike_net") -> bool:
        body = {"Container": container, "Force": True}
        status, _ = self._request("POST", f"/networks/{urllib.parse.quote(network)}/disconnect", body)
        return status in (200, 204)

    def connect_network(self, container: str, network: str = "ha_spike_net") -> bool:
        body = {"Container": container}
        status, _ = self._request("POST", f"/networks/{urllib.parse.quote(network)}/connect", body)
        return status in (200, 204)

    def get_stats(self, name: str) -> Optional[Dict[str, Any]]:
        status, data = self._request("GET", f"/containers/{urllib.parse.quote(name)}/stats?stream=false")
        if status == 200 and isinstance(data, dict):
            return data
        return None
