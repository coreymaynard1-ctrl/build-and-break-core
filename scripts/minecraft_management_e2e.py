#!/usr/bin/env python3
"""Real Minecraft management test with a revoked temporary administrator.
Creates only its own service. Never changes an existing customer's workload.
Leaves its test service stopped and preserves its world for inspection.
"""
import json, os, secrets, socket, struct, subprocess, time, urllib.request, uuid
import lifecycle_e2e as lifecycle
from authenticated_e2e import sql
from minecraft_e2e import varint, read_varint, recv_exact

def ping(port):
    with socket.create_connection(('127.0.0.1', port), timeout=3) as s:
        host = b'localhost'
        packet = b'\x00' + varint(767) + varint(len(host)) + host + struct.pack('>H', port) + b'\x01'
        s.sendall(varint(len(packet)) + packet + b'\x01\x00')
        if read_varint(s) > 1048576: raise ValueError('oversized status')
        assert read_varint(s) == 0
        length = read_varint(s)
        if length > 1048576: raise ValueError('oversized status JSON')
        return json.loads(recv_exact(s, length))

def ready(port, motd, timeout=420):
    end = time.monotonic() + timeout
    while time.monotonic() < end:
        try:
            result = ping(port)
            if motd in json.dumps(result.get('description')): return result
        except (OSError, ValueError, AssertionError, ConnectionError): pass
        time.sleep(3)
    raise AssertionError('Minecraft did not finish installation and answer protocol status')

def main():
    if not os.environ.get('DATABASE_URL'): raise RuntimeError('Load .env first')
    suffix = str(uuid.uuid4())
    body = {'email': 'bnb-mc-e2e-' + suffix + '@example.test', 'name': 'Temporary Minecraft test', 'password': secrets.token_urlsafe(32)}
    req = urllib.request.Request(lifecycle.BASE + '/auth/register', data=json.dumps(body).encode(), headers={'Content-Type': 'application/json'})
    with lifecycle.OPENER.open(req, timeout=15) as res: session = json.load(res)
    actor = str(uuid.UUID(session['user']['id'])); sid = None
    try:
        sql("UPDATE accounts SET role='admin' WHERE id='" + actor + "'")
        lifecycle.CSRF = session['csrf_token']
        if os.environ.get('BNB_MINECRAFT_RESUME_TEST') == '1':
            with open('/tmp/bnb-minecraft-management-test.json') as f: saved = json.load(f)
            sid = str(uuid.UUID(saved['service_id'])); server = {'id': str(uuid.UUID(saved['server_id']))}; port = saved['port']
            existing = next(s for s in lifecycle.request('/services') if s['id'] == sid)
            assert existing['name'].startswith('Minecraft management test ') and existing['server_id'] == server['id']
            existing_config = lifecycle.request('/services/' + sid + '/minecraft')['requested']
            if existing_config['type'] != 'PAPER':
                existing_config.update({'type': 'PAPER', 'modpack': '', 'projects': ['luckperms'], 'build': '', 'java': 'auto'})
                lifecycle.request('/services/' + sid + '/minecraft', 'PUT', existing_config, 202)
            elif existing['status'] == 'stopped':
                lifecycle.request('/services/' + sid + '/start', 'POST', expected=202)
            original = lifecycle.wait_service(sid, 'running', 660)['runtime_id']
            ready(port, '')
        else:
            server = lifecycle.request('/servers', 'POST', {'node_id': lifecycle.NODE, 'name': 'Minecraft management test ' + suffix[:8], 'memory_mb': 2048, 'disk_mb': 4096, 'cpu_threads': 2}, 201)
            port = 25585
            service = lifecycle.request('/services', 'POST', {'server_id': server['id'], 'name': 'Minecraft management test ' + suffix[:8], 'template': 'minecraft-java', 'memory_mb': 2048, 'cpu_threads': 2, 'port': port, 'accept_eula': True}, 202)
            sid = service['id']; original = lifecycle.wait_service(sid, 'running', 660)['runtime_id']
            ready(port, 'A Minecraft Server')
        subprocess.run(['docker', 'exec', original, 'sh', '-c', 'printf management-test > /data/.bnb-management-e2e'], check=True, stdout=subprocess.DEVNULL)
        print('PASS initial Minecraft protocol status', flush=True)
        config = lifecycle.request('/services/' + sid + '/minecraft')['requested']
        config.update({'motd': 'Build and Break management verification', 'type': 'PAPER', 'projects': ['luckperms']})
        lifecycle.request('/services/' + sid + '/minecraft', 'PUT', config, 202)
        current = lifecycle.wait_service(sid, 'running', 660)
        assert current['runtime_id'] != original
        ready(port, config['motd'])
        marker = subprocess.check_output(['docker', 'exec', current['runtime_id'], 'cat', '/data/.bnb-management-e2e'], text=True)
        assert marker == 'management-test'
        plugins = subprocess.check_output(['docker', 'exec', current['runtime_id'], 'find', '/data/plugins', '-maxdepth', '1', '-iname', '*luckperms*.jar'], text=True)
        assert plugins.strip()
        print('PASS Paper JAR selection, LuckPerms file, retained world marker and changed MOTD', flush=True)
        lifecycle.request('/services/' + sid + '/minecraft/command', 'POST', {'command': 'say BuildAndBreakConsoleVerification'}, 202)
        end = time.monotonic() + 90
        while time.monotonic() < end:
            current = next(s for s in lifecycle.request('/services') if s['id'] == sid)
            if 'BuildAndBreakConsoleVerification' in (current.get('console_tail') or ''): break
            time.sleep(2)
        else: raise AssertionError('Minecraft command was not delivered into console output')
        print('PASS Minecraft command and agent console collection', flush=True)
        config['projects'] = []
        lifecycle.request('/services/' + sid + '/minecraft', 'PUT', config, 202)
        current = lifecycle.wait_service(sid, 'running', 660); ready(port, config['motd'])
        plugins = subprocess.check_output(['docker', 'exec', current['runtime_id'], 'find', '/data/plugins', '-maxdepth', '1', '-iname', '*luckperms*.jar'], text=True)
        assert not plugins.strip()
        print('PASS plugin removal applied to the real persistent volume', flush=True)
        config.update({'type': 'FABRIC', 'build': '', 'projects': ['fabric-api', 'lithium']})
        lifecycle.request('/services/' + sid + '/minecraft', 'PUT', config, 202)
        current = lifecycle.wait_service(sid, 'running', 660); ready(port, config['motd'])
        mods = subprocess.check_output(['docker', 'exec', current['runtime_id'], 'find', '/data/mods', '-maxdepth', '1', '-name', '*.jar'], text=True)
        assert 'fabric-api' in mods.lower() and 'lithium' in mods.lower()
        assert subprocess.check_output(['docker', 'exec', current['runtime_id'], 'cat', '/data/.bnb-management-e2e'], text=True) == 'management-test'
        print('PASS Fabric loader, Fabric API/Lithium installation and persistent data marker', flush=True)
        packs = lifecycle.request('/services/' + sid + '/minecraft/catalogue?type=MODRINTH&version=1.21.1&q=adrenaline')['hits']
        pack = next((p for p in packs if p['slug'] == 'adrenaline'), None)
        if pack:
            config.update({'type': 'MODRINTH', 'projects': [], 'modpack': pack['slug']})
            lifecycle.request('/services/' + sid + '/minecraft', 'PUT', config, 202)
            current = lifecycle.wait_service(sid, 'running', 660); ready(port, '')
            assert subprocess.check_output(['docker', 'exec', current['runtime_id'], 'cat', '/data/.bnb-management-e2e'], text=True) == 'management-test'
            print('PASS compatible Modrinth pack installation and game-protocol readiness', flush=True)
        else:
            print('Modrinth pack smoke not run: selected reference test pack has no compatible catalogue result', flush=True)
        # Restore the known Paper configuration for browser/operator inspection.
        config.update({'type': 'PAPER', 'projects': ['luckperms'], 'modpack': ''})
        lifecycle.request('/services/' + sid + '/minecraft', 'PUT', config, 202)
        lifecycle.wait_service(sid, 'running', 660); ready(port, config['motd'])
        lifecycle.request('/services/' + sid + '/stop', 'POST', expected=202)
        lifecycle.wait_service(sid, 'stopped', 120)
        # Identifiers alone may be used by operator/browser checks; no credentials saved.
        with open('/tmp/bnb-minecraft-management-test.json', 'w') as f:
            json.dump({'service_id': sid, 'server_id': server['id'], 'port': port}, f)
        print('PASS test service left stopped with persistent world retained', flush=True)
    finally:
        if sid:
            try:
                current = next((s for s in lifecycle.request('/services') if s['id'] == sid), None)
                if current and current['status'] == 'running' and current['job_status'] not in ('pending', 'processing'):
                    lifecycle.request('/services/' + sid + '/stop', 'POST', expected=202)
                    lifecycle.wait_service(sid, 'stopped', 120)
            except Exception: print('Test workload needs operator cleanup; unrelated workloads were untouched', flush=True)
        sql("UPDATE accounts SET role='customer',disabled=true WHERE id='" + actor + "'")
        sql("DELETE FROM account_sessions WHERE account_id='" + actor + "'")
        print('PASS temporary Minecraft test admin revoked', flush=True)

if __name__ == '__main__': main()
