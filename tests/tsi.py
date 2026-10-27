#!/usr/bin/env python3
"""TSI sockets and session lifecycle; all network fixtures run on this host."""
import json
import os
from pathlib import Path
import platform
import select
import signal
import socket
import socketserver
import struct
import subprocess
import tempfile
import threading
import time
import uuid

repo = Path(__file__).resolve().parents[1]
binary = str(repo / 'out/revm/bin/revm')

class EchoTCP(socketserver.BaseRequestHandler):
    def handle(self):
        while data := self.request.recv(4096):
            self.request.sendall(data)

class EchoUDP(socketserver.BaseRequestHandler):
    def handle(self):
        data, sock = self.request
        sock.sendto(data, self.client_address)

class DNS(socketserver.BaseRequestHandler):
    def handle(self):
        data, sock = self.request
        header = data[:2] + struct.pack('!HHHHH', 0x8180, 1, 1, 0, 0)
        answer = b'\xc0\x0c' + struct.pack('!HHIH', 1, 1, 60, 4) + socket.inet_aton('192.0.2.42')
        end = 12
        while data[end]:
            end += data[end] + 1
        question = data[12:end + 5]
        sock.sendto(header + question + answer, self.client_address)

with tempfile.TemporaryDirectory(prefix='revm-tsi-') as root:
    root = Path(root)
    fixtures = root / 'fixtures'
    fixtures.mkdir()
    arch = {'arm64': 'arm64', 'aarch64': 'arm64', 'x86_64': 'amd64'}[platform.machine()]
    subprocess.run(['go', 'build', '-o', str(fixtures / 'probe'), './tests/tsi'], cwd=repo,
                   env=dict(os.environ, GOOS='linux', GOARCH=arch, CGO_ENABLED='0'), check=True, timeout=90)
    env = dict(os.environ, XDG_STATE_HOME=str(root / 'state'),
               XDG_CACHE_HOME=str(root / 'cache'), XDG_DATA_HOME=str(root / 'data'))
    processes, servers = [], []

    def cli(args, code=0, data=None):
        p = subprocess.run([binary] + args, env=env, input=data, capture_output=True, timeout=20)
        assert p.returncode == code, (args, p.returncode, p.stdout, p.stderr)
        return p

    def start(args):
        p = subprocess.Popen([binary] + args, env=env, stdin=subprocess.DEVNULL,
                             stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        processes.append(p)
        assert select.select([p.stdout], [], [], 20)[0], 'no readiness output'
        assert p.stdout.readline() == b'ready\n', p.communicate(timeout=5)
        return p

    def fixture(family, udp=False, handler=None, host=None):
        base = socketserver.UDPServer if udp else socketserver.ThreadingTCPServer
        server_type = type('Server', (base,), {'address_family': family, 'daemon_threads': True})
        host = host or ('127.0.0.1' if family == socket.AF_INET else '::1')
        server = server_type((host, 0), handler or (EchoUDP if udp else EchoTCP))
        servers.append(server)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        port = server.server_address[1]
        return host, port, f'{host}:{port}' if family == socket.AF_INET else f'[{host}]:{port}'

    try:
        streamed = cli(['run', '--network', 'tsi', '--cpus', '1', '--memory', '512', '--',
             'sh', '-c', 'cat; printf diagnostic >&2; exit 42'], 42, b'stream\x00\xff\n')
        assert streamed.stdout == b'stream\x00\xff\n' and streamed.stderr == b'diagnostic'
        owner = start(['run', '--network', 'tsi', '--id', 'tsi', '--cpus', '1', '--memory', '512',
                       '--mount', str(fixtures) + ':/tests,ro', '--', 'sh', '-c', 'echo ready; sleep 100'])
        cli(['wait', '--ready', '--timeout', '5s', 'tsi'])
        view = json.loads(cli(['inspect', 'tsi']).stdout)
        assert view['config']['network'] == 'tsi'
        runtime = Path(view['lastRun']['runtimeDir'])
        assert not list(runtime.glob('**/gvp*')) and not list(runtime.glob('**/vnet.sock'))
        command = ['exec', '--id', 'tsi', '--']
        expected_dns = Path('/etc/resolv.conf').read_text()
        expected_dns = '\n'.join(line for line in expected_dns.split('\n')
            if not (len(line.split()) >= 2 and line.split()[0] == 'nameserver' and '%' in line.split()[1]))
        assert cli(command + ['cat', '/etc/resolv.conf']).stdout.decode() == expected_dns
        cli(command + ['sh', '-c', 'test ! -e /sys/class/net/eth0 && test ! -e /etc/containers/podman-machine'])
        assert b'TSI' in cli(['ports', 'tsi'], 1).stderr
        print('PASS TSI boot, DNS configuration, streaming and session readiness without gvproxy', flush=True)

        families = [socket.AF_INET]
        try:
            with socket.socket(socket.AF_INET6) as s:
                s.bind(('::1', 0))
            families.append(socket.AF_INET6)
        except OSError:
            print('SKIP IPv6: host loopback is unavailable', flush=True)
        for family in families:
            suffix = '4' if family == socket.AF_INET else '6'
            host, _, address = fixture(family)
            cli(command + ['/tests/probe', 'client', 'tcp' + suffix, address])
            _, _, udp_address = fixture(family, udp=True)
            cli(command + ['/tests/probe', 'datagram', 'udp' + suffix, udp_address])
            with socket.socket(family) as s:
                s.bind((host, 0))
                port = s.getsockname()[1]
            listen = f'{host}:{port}' if suffix == '4' else f'[{host}]:{port}'
            attached = start(command + ['/tests/probe', 'serve', 'tcp' + suffix, listen])
            with socket.create_connection((host, port), timeout=5) as c:
                c.sendall(b'host-to-guest\n')
                with c.makefile('rb') as response:
                    assert response.readline() == b'host-to-guest\n'
            attached.send_signal(signal.SIGTERM)
            attached.communicate(timeout=5)
            assert attached.returncode == 143
            # Disconnect must remove the guest listener from the host too.
            deadline = time.monotonic() + 5
            while True:
                try:
                    with socket.create_connection((host, port), timeout=.2):
                        pass
                except OSError:
                    break
                assert time.monotonic() < deadline, 'listener survived exec cancellation'
                time.sleep(.05)
            print(f'PASS TSI IPv{suffix} TCP/UDP, direct listening and cancellation', flush=True)

        # Connected UDP to guest loopback is intercepted by the guest IP stack
        # in the pinned TSI kernel. Use a non-loopback host address for this path.
        host_ip = None
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as s:
            try:
                s.connect(('192.0.2.1', 9))  # Select a route; no packets are sent.
                host_ip = s.getsockname()[0]
            except OSError:
                pass
        if host_ip and not host_ip.startswith('127.'):
            _, _, udp_address = fixture(socket.AF_INET, udp=True, host=host_ip)
            cli(command + ['/tests/probe', 'client', 'udp4', udp_address])
            _, _, dns_address = fixture(socket.AF_INET, udp=True, handler=DNS, host=host_ip)
            cli(command + ['/tests/probe', 'dns', dns_address])
            print('PASS connected UDP and DNS through host network', flush=True)
        else:
            print('SKIP connected UDP/DNS: no non-loopback host IPv4 address', flush=True)
        unix_path = '/tmp/revm-tsi-' + uuid.uuid4().hex + '.sock'
        cli(command + ['/tests/probe', 'unix', unix_path])
        assert not os.path.exists(unix_path)
        cli(['stop', '--timeout', '6s', 'tsi'])
        owner.communicate(timeout=5)
        assert not runtime.exists()
        cli(['rm', 'tsi'])
        print('PASS guest-local Unix sockets and stop cleanup', flush=True)

        with socket.socket() as s:
            s.bind(('127.0.0.1', 0))
            port = s.getsockname()[1]
        killed = start(['run', '--network', 'tsi', '--id', 'killed', '--cpus', '1', '--memory', '512',
                        '--mount', str(fixtures) + ':/tests,ro', '--',
                        '/tests/probe', 'serve', 'tcp4', f'127.0.0.1:{port}'])
        runtime = Path(json.loads(cli(['inspect', 'killed']).stdout)['lastRun']['runtimeDir'])
        with socket.create_connection(('127.0.0.1', port), timeout=5):
            pass
        killed.kill()
        killed.communicate(timeout=5)
        cli(['wait', '--timeout', '6s', 'killed'], 1)
        assert not runtime.exists()
        try:
            with socket.create_connection(('127.0.0.1', port), timeout=1):
                raise AssertionError('TSI listener survived VM SIGKILL')
        except ConnectionRefusedError:
            pass
        cli(['rm', 'killed'])
        print('PASS TSI host listener and runtime cleanup after SIGKILL', flush=True)
    finally:
        for p in processes:
            if p.poll() is None:
                p.kill()
            p.communicate(timeout=5)
        subprocess.run([binary, 'cache', 'prune'], env=env, capture_output=True, timeout=10)
        for server in servers:
            server.shutdown()
            server.server_close()
