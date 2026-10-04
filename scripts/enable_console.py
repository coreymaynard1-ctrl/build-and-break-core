#!/usr/bin/env python3
"""Operator migration: activate console input on one explicitly selected service.
Preserves the applied workload and world; revokes the temporary operator account.
"""
import argparse, json, re, secrets, urllib.request, uuid
import lifecycle_e2e as lifecycle
from authenticated_e2e import sql
from minecraft_management_e2e import ready

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--service', required=True)
    parser.add_argument('--owner', required=True)
    args = parser.parse_args()
    sid = str(uuid.UUID(args.service))
    if not re.fullmatch(r'[A-Za-z0-9_.+-]+@[A-Za-z0-9.-]+', args.owner): raise ValueError('invalid owner email')
    # Refuse to restart an unrelated service. Values are constrained UUID/email literals.
    sql("DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM services s JOIN servers v ON v.id=s.server_id JOIN accounts a ON a.id=v.account_id WHERE s.id='" + sid + "' AND a.email='" + args.owner + "' AND s.template='minecraft-java' AND s.deleted_at IS NULL) THEN RAISE EXCEPTION 'selected service does not belong to this owner'; END IF; END $$")
    body = {'email': 'bnb-console-operator-' + str(uuid.uuid4()) + '@example.test', 'name': 'Temporary console migration operator', 'password': secrets.token_urlsafe(32)}
    req = urllib.request.Request(lifecycle.BASE + '/auth/register', data=json.dumps(body).encode(), headers={'Content-Type': 'application/json'})
    with lifecycle.OPENER.open(req, timeout=15) as res: session = json.load(res)
    actor = str(uuid.UUID(session['user']['id']))
    try:
        sql("UPDATE accounts SET role='admin' WHERE id='" + actor + "'")
        lifecycle.CSRF = session['csrf_token']
        service = next(s for s in lifecycle.request('/services') if s['id'] == sid)
        if not service['console_input_enabled']:
            lifecycle.request('/services/' + sid + '/minecraft/console', 'POST', expected=202)
            service = lifecycle.wait_service(sid, 'running', 180)
        assert service['console_input_enabled']
        ready(service['port'], '', timeout=180)
        lifecycle.request('/services/' + sid + '/minecraft/command', 'POST', {'command': 'say BuildAndBreak console input is ready'}, 202)
        lifecycle.wait_service(sid, 'running', 60)
        print('PASS selected server console input activated and Minecraft command delivered', flush=True)
    finally:
        sql("UPDATE accounts SET role='customer',disabled=true WHERE id='" + actor + "'")
        sql("DELETE FROM account_sessions WHERE account_id='" + actor + "'")
        print('PASS temporary migration operator revoked', flush=True)

if __name__ == '__main__': main()
