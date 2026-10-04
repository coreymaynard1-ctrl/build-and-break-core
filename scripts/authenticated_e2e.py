#!/usr/bin/env python3
"""Operator-only development verification using a temporary test admin.
Requires DATABASE_URL, Core, the enrolled agent and Docker. Creates only test
resources. Revokes the temporary account and session when verification ends.
No passwords, credentials or cookies are printed or saved.
"""
import json, os, secrets, subprocess, urllib.parse, urllib.request, uuid
import lifecycle_e2e as lifecycle
import billing_e2e

def sql(query):
    connection = urllib.parse.urlsplit(os.environ['DATABASE_URL'])
    env = {**os.environ, 'PGHOST': connection.hostname or '127.0.0.1', 'PGPORT': str(connection.port or 5432), 'PGDATABASE': urllib.parse.unquote(connection.path.lstrip('/')), 'PGUSER': urllib.parse.unquote(connection.username or ''), 'PGPASSWORD': urllib.parse.unquote(connection.password or '')}
    options = urllib.parse.parse_qs(connection.query)
    if options.get('sslmode'): env['PGSSLMODE'] = options['sslmode'][0]
    result = subprocess.run(['psql', '-X', '-v', 'ON_ERROR_STOP=1', '-c', query], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
    if result.returncode: raise RuntimeError('Test account database operation failed')

def main():
    if not os.environ.get('DATABASE_URL'): raise RuntimeError('Load .env before running this operator test')
    email = 'bnb-e2e-' + str(uuid.uuid4()) + '@example.test'
    body = {'email': email, 'name': 'Temporary lifecycle test operator', 'password': secrets.token_urlsafe(32)}
    req = urllib.request.Request(lifecycle.BASE + '/auth/register', data=json.dumps(body).encode(), headers={'Content-Type': 'application/json'})
    with lifecycle.OPENER.open(req, timeout=15) as response: session = json.load(response)
    actor = str(uuid.UUID(session['user']['id']))
    try:
        sql("UPDATE accounts SET role='admin' WHERE id='" + actor + "'")
        lifecycle.CSRF = session['csrf_token']
        lifecycle.main()
        billing_e2e.main()
    finally:
        sql("UPDATE accounts SET role='customer',disabled=true WHERE id='" + actor + "'")
        sql("DELETE FROM account_sessions WHERE account_id='" + actor + "'")
        print('PASS temporary test administrator access revoked', flush=True)

if __name__ == '__main__': main()
