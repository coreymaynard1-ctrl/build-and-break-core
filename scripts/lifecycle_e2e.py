#!/usr/bin/env python3
"""Real local lifecycle test. Creates only its own allocation/service.
No credentials are read and no pre-existing container is modified.
Requires Core and upgraded enrolled agent running on localhost.
"""
import json, os, time, urllib.request, urllib.error, uuid
BASE = os.environ.get('BNB_CORE_URL', 'http://127.0.0.1:8080') + '/api/v1'
NODE = os.environ.get('BNB_NODE_ID', 'c6076de5-90d4-4842-b903-df48d453637c')
def request(path, method='GET', body=None, expected=200, headers=None):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(BASE + path, data=data, method=method, headers={'Content-Type': 'application/json', **(headers or {})})
    try:
        with urllib.request.urlopen(req, timeout=15) as res:
            assert res.status == expected, (res.status, expected)
            return json.load(res)
    except urllib.error.HTTPError as err:
        if err.code == expected: return None
        raise AssertionError(f'{method} {path}: HTTP {err.code}: {err.read().decode()}') from None
def wait_service(service_id, state, timeout=90):
    end=time.monotonic()+timeout
    while time.monotonic()<end:
        services=request('/services')
        found=next((s for s in services if s['id']==service_id),None)
        if state=='deleted' and found is None: return None
        if found and found['status']==state and found['job_status']=='completed': return found
        if found and found['status']=='error': raise AssertionError(found['error_message'])
        time.sleep(1)
    raise AssertionError(f'timed out waiting for {state}')
def main():
    suffix=str(uuid.uuid4())[:8]
    request('/services','POST',{'server_id':'invalid'},400)
    request('/services/not-a-uuid/start','POST',expected=400)
    request('/nodes/'+NODE+'/jobs/claim','POST',expected=401)
    server=request('/servers','POST',{'node_id':NODE,'name':'E2E allocation '+suffix,'memory_mb':128,'disk_mb':128,'cpu_threads':1},201)
    service=request('/services','POST',{'server_id':server['id'],'name':'E2E Alpine '+suffix,'template':'alpine-test','memory_mb':64,'cpu_threads':1},202)
    sid=service['id']; running=wait_service(sid,'running'); rid=running['runtime_id']; assert rid
    print('PASS create: Core -> PostgreSQL queue -> agent -> Docker -> persisted runtime ID',flush=True)
    for action,state in [('stop','stopped'),('start','running'),('restart','running')]:
        request('/services/'+sid+'/'+action,'POST',expected=202)
        current=wait_service(sid,state);assert current['runtime_id']==rid
        print('PASS '+action+': state and stable runtime identity',flush=True)
    request('/services/'+sid,'DELETE',expected=202);wait_service(sid,'deleted')
    jobs=request('/jobs?service_id='+sid);assert len(jobs)==5 and all(j['status']=='completed' for j in jobs)
    print('PASS delete: service tombstone and five completed jobs',flush=True)
    print(json.dumps({'server_id':server['id'],'service_id':sid,'runtime_id':rid}),flush=True)
    # Allocation/job history remains available for audit. Delete only test-owned container through Core.
if __name__=='__main__': main()
