#!/usr/bin/env python3
"""Real sandbox billing, Bedrock UDP readiness and persistent lifecycle test."""
import json, secrets, socket, struct, subprocess, time, urllib.request, uuid
import lifecycle_e2e as lifecycle
from authenticated_e2e import sql

def ready(port,timeout=360):
    magic=bytes.fromhex('00ffff00fefefefefdfdfdfd12345678')
    end=time.monotonic()+timeout
    while time.monotonic()<end:
        try:
            with socket.socket(socket.AF_INET,socket.SOCK_DGRAM) as sock:
                sock.settimeout(2)
                with urllib.request.urlopen('http://127.0.0.1:'+str(port)+'/v1/join',timeout=2) as response:
                    data=json.load(response)
                    if data.get('version') and response.status==200: return
        except (OSError, ValueError): pass
        time.sleep(3)
    raise AssertionError('Bedrock did not answer UDP status')

def main():
    body={'email':'bnb-bedrock-'+str(uuid.uuid4())+'@example.test','name':'Temporary Bedrock test','password':secrets.token_urlsafe(32)}
    req=urllib.request.Request(lifecycle.BASE+'/auth/register',data=json.dumps(body).encode(),headers={'Content-Type':'application/json'})
    with lifecycle.OPENER.open(req,timeout=15) as res: session=json.load(res)
    actor=str(uuid.UUID(session['user']['id']))
    try:
        sql("UPDATE accounts SET role='admin' WHERE id='"+actor+"'")
        lifecycle.CSRF=session['csrf_token']
        import os
        if os.path.exists('/tmp/bnb-bedrock-test.json') and os.environ.get('BNB_BEDROCK_RESUME_TEST')!='1':
            with open('/tmp/bnb-bedrock-test.json') as f: old=json.load(f)
            old_id=str(uuid.UUID(old['service_id']));old_server=str(uuid.UUID(old['server_id']))
            previous=next((s for s in lifecycle.request('/services') if s['id']==old_id),None)
            if previous:
                assert previous['name'].startswith('Bedrock deployment test ') and previous['server_id']==old_server
                lifecycle.request('/services/'+old_id,'DELETE',expected=202)
                lifecycle.wait_service(old_id,'deleted',90)
                lifecycle.request('/servers/'+old_server,'DELETE')
        # Retire only explicitly identified session-created test allocations.
        for allocation_id, service_id in [('1cef2558-80be-4a14-9cf1-ea467145adfb','bcf2ee70-ddcb-407a-ba94-d707e55b8cee'),('2e0d0bd3-451f-4255-bd86-ca86406c45ba','b2ad5491-6413-4636-81dc-1448d1b8b958'),('9d072d5c-949a-43c1-b21a-535f6aa6eb6c','a481af90-9409-4c2d-8bd6-f2c3a7e095c0')]:
            allocations = lifecycle.request('/servers')
            old = next((s for s in allocations if s['id']==allocation_id),None)
            if old is None: continue
            assert old['name'].startswith(('E2E allocation ', 'Billing E2E '))
            workload = next((s for s in lifecycle.request('/services') if s['id']==service_id),None)
            if workload is not None:
                assert workload['server_id']==allocation_id and workload['status']=='stopped' and workload['name'].startswith('Billing E2E ')
                lifecycle.request('/services/'+service_id,'DELETE',expected=202)
                lifecycle.wait_service(service_id,'deleted',90)
            lifecycle.request('/servers/'+allocation_id,'DELETE')
        if os.environ.get('BNB_BEDROCK_RESUME_TEST')=='1':
            with open('/tmp/bnb-bedrock-test.json') as f: saved=json.load(f)
            sid=str(uuid.UUID(saved['service_id']))
            current=next(s for s in lifecycle.request('/services') if s['id']==sid)
            assert current['name'].startswith('Bedrock deployment test ') and len(current['extra_ports'])==16
            if current['status']=='stopped': lifecycle.request('/services/'+sid+'/start','POST',expected=202)
            current=lifecycle.wait_service(sid,'running',90)
        else:
            order=lifecycle.request('/billing/orders','POST',{'plan_id':'dev-bedrock','template':'minecraft-bedrock','node_id':lifecycle.NODE,'name':'Bedrock deployment test '+actor[:8],'email':body['email'],'accept_eula':True},201,{'Idempotency-Key':str(uuid.uuid4())})
            paid=lifecycle.request('/billing/invoices/'+order['invoice_id']+'/pay-sandbox','POST')
            sid=paid['service_id']; current=lifecycle.wait_service(sid,'running',360)
            with open('/tmp/bnb-bedrock-test.json','w') as f: json.dump({'service_id':sid,'server_id':paid['server_id'],'port':current['port']},f)
        ready(current['port'])
        subprocess.check_call(['docker','exec',current['runtime_id'],'sh','-c','printf verified > /data/.bnb-bedrock-marker'])
        print('PASS sandbox payment provisions Bedrock and answers real NetherNet HTTP status',flush=True)
        lifecycle.request('/services/'+sid+'/restart','POST',expected=202)
        current=lifecycle.wait_service(sid,'running',90);ready(current['port'])
        assert subprocess.check_output(['docker','exec',current['runtime_id'],'cat','/data/.bnb-bedrock-marker'])==b'verified'
        lifecycle.request('/services/'+sid+'/stop','POST',expected=202)
        lifecycle.wait_service(sid,'stopped',90)
        print('PASS Bedrock restart preserves data; dedicated test server stopped',flush=True)
    finally:
        sql("UPDATE accounts SET role='customer',disabled=true WHERE id='"+actor+"'")
        sql("DELETE FROM account_sessions WHERE account_id='"+actor+"'")
        print('PASS temporary Bedrock test account revoked',flush=True)

if __name__=='__main__':main()
