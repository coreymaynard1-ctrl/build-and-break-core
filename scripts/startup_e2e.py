#!/usr/bin/env python3
"""Verify custom startup only on the dedicated management test service."""
import json, secrets, subprocess, urllib.request, uuid
import lifecycle_e2e as lifecycle
from authenticated_e2e import sql
from minecraft_management_e2e import ready

def main():
    with open('/tmp/bnb-minecraft-management-test.json') as f: saved = json.load(f)
    sid = str(uuid.UUID(saved['service_id']))
    body = {'email': 'bnb-startup-' + str(uuid.uuid4()) + '@example.test', 'name': 'Temporary startup test', 'password': secrets.token_urlsafe(32)}
    req = urllib.request.Request(lifecycle.BASE + '/auth/register', data=json.dumps(body).encode(), headers={'Content-Type': 'application/json'})
    with lifecycle.OPENER.open(req, timeout=15) as res: session = json.load(res)
    actor = str(uuid.UUID(session['user']['id']))
    original = None
    try:
        sql("UPDATE accounts SET role='admin' WHERE id='" + actor + "'")
        lifecycle.CSRF = session['csrf_token']
        service = next(s for s in lifecycle.request('/services') if s['id'] == sid)
        assert service['name'].startswith('Minecraft management test ') and service['port'] == saved['port']
        data = lifecycle.request('/services/' + sid + '/minecraft')
        assert 'java ' in data['egg']['default_startup']
        original = data['requested']
        config = dict(original, startup='env BNB_STARTUP_TEST=verified {{DEFAULT_COMMAND}}')
        lifecycle.request('/services/' + sid + '/minecraft', 'PUT', config, 202)
        current = lifecycle.wait_service(sid, 'running', 180)
        ready(saved['port'], '', timeout=240)
        output = subprocess.check_output(['docker', 'exec', current['runtime_id'], 'sh', '-c', 'for p in /proc/[0-9]*/environ; do cat "$p" 2>/dev/null; done'], stderr=subprocess.DEVNULL)
        assert b'BNB_STARTUP_TEST=verified' in output
        lifecycle.request('/services/' + sid + '/minecraft/command', 'POST', {'command': 'say BuildAndBreak custom startup verified'}, 202)
        lifecycle.wait_service(sid, 'running', 60)
        print('PASS actual game startup uses edited command, protocol responds, console command succeeds', flush=True)
        config['startup'] = '{{DEFAULT_COMMAND}}'
        lifecycle.request('/services/' + sid + '/minecraft', 'PUT', config, 202)
        lifecycle.wait_service(sid, 'running', 180)
        ready(saved['port'], '', timeout=240)
        lifecycle.request('/services/' + sid + '/stop', 'POST', expected=202)
        lifecycle.wait_service(sid, 'stopped', 90)
        print('PASS template default restored and dedicated test server stopped', flush=True)
    finally:
        sql("UPDATE accounts SET role='customer',disabled=true WHERE id='" + actor + "'")
        sql("DELETE FROM account_sessions WHERE account_id='" + actor + "'")
        print('PASS temporary startup operator revoked', flush=True)

if __name__ == '__main__': main()
