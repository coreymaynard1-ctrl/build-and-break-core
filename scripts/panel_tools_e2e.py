#!/usr/bin/env python3
"""Real file-manager and Bedrock addon operations on dedicated test volumes."""
import base64, io, json, secrets, time, urllib.request, uuid, zipfile
import lifecycle_e2e as lifecycle
from authenticated_e2e import sql
from bedrock_e2e import ready

def operation(path,body):
    queued=lifecycle.request(path,'POST',body,202)
    return wait(queued['job_id'])

def wait(job_id):
    end=time.monotonic()+90
    while time.monotonic()<end:
        job=lifecycle.request('/jobs/'+job_id)
        if job['status']=='failed': raise AssertionError(job['error_message'])
        if job['status']=='completed': return json.loads(job.get('result',{}).get('data') or '{}')
        time.sleep(.4)
    raise AssertionError('panel operation timed out')

def main():
    with open('/tmp/bnb-minecraft-management-test.json') as f: java=json.load(f)
    with open('/tmp/bnb-bedrock-test.json') as f: bedrock=json.load(f)
    body={'email':'bnb-panel-tools-'+str(uuid.uuid4())+'@example.test','name':'Temporary panel test','password':secrets.token_urlsafe(32)}
    req=urllib.request.Request(lifecycle.BASE+'/auth/register',data=json.dumps(body).encode(),headers={'Content-Type':'application/json'})
    with lifecycle.OPENER.open(req,timeout=15) as res: session=json.load(res)
    actor=str(uuid.UUID(session['user']['id']))
    try:
        sql("UPDATE accounts SET role='admin' WHERE id='"+actor+"'")
        lifecycle.CSRF=session['csrf_token']
        for saved,prefix in [(java,'Minecraft management test '),(bedrock,'Bedrock deployment test ')]:
            sid=str(uuid.UUID(saved['service_id']))
            server=next(s for s in lifecycle.request('/services') if s['id']==sid)
            assert server['name'].startswith(prefix) and server['status']=='stopped'
            path='/services/'+sid+'/files';folder='.bnb-file-test-'+actor[:8]
            operation(path,{'operation':'mkdir','path':folder})
            value=base64.b64encode(b'owned persistent file').decode()
            operation(path,{'operation':'write','path':folder+'/example.txt','content':value})
            assert operation(path,{'operation':'read','path':folder+'/example.txt'})['content']==value
            operation(path,{'operation':'rename','path':folder+'/example.txt','destination':folder+'/renamed.txt'})
            assert operation(path,{'operation':'list','path':folder})['entries'][0]['name']=='renamed.txt'
            operation(path,{'operation':'delete','path':folder+'/renamed.txt'})
            operation(path,{'operation':'delete','path':folder})
        print('PASS list/write/read/rename/delete on both stopped game volumes',flush=True)
        sid=str(uuid.UUID(bedrock['service_id']));pack_id=str(uuid.uuid4())
        manifest={'format_version':2,'header':{'name':'Build & Break addon test','description':'Dedicated runtime verification pack','uuid':pack_id,'version':[1,0,0],'min_engine_version':[1,26,0]},'modules':[{'type':'data','uuid':str(uuid.uuid4()),'version':[1,0,0]}]}
        archive=io.BytesIO()
        with zipfile.ZipFile(archive,'w',zipfile.ZIP_DEFLATED) as z:
            z.writestr('manifest.json',json.dumps(manifest));z.writestr('functions/bnb_test.mcfunction','say BuildAndBreak addon ready')
        req=urllib.request.Request(lifecycle.BASE+'/services/'+sid+'/bedrock/addons',data=archive.getvalue(),headers={'Content-Type':'application/octet-stream','X-CSRF-Token':lifecycle.CSRF},method='POST')
        with lifecycle.OPENER.open(req,timeout=15) as res: queued=json.load(res);assert res.status==202
        wait(queued['job_id'])
        pack=next(p for p in lifecycle.request('/services/'+sid+'/bedrock/addons') if p['pack_id']==pack_id)
        assert pack['state']=='installed' and not pack['applied_enabled']
        queued=lifecycle.request('/services/'+sid+'/bedrock/addons/'+pack['id'],'PUT',{'enabled':True},202);wait(queued['job_id'])
        world=operation('/services/'+sid+'/files',{'operation':'read','path':'worlds/Bedrock level/world_behavior_packs.json'})
        assert pack_id in base64.b64decode(world['content']).decode()
        lifecycle.request('/services/'+sid+'/start','POST',expected=202)
        current=lifecycle.wait_service(sid,'running',90);ready(current['port'])
        lifecycle.request('/services/'+sid+'/stop','POST',expected=202);lifecycle.wait_service(sid,'stopped',90)
        queued=lifecycle.request('/services/'+sid+'/bedrock/addons/'+pack['id'],'PUT',{'enabled':False},202);wait(queued['job_id'])
        packs=lifecycle.request('/services/'+sid+'/bedrock/addons');assert not next(p for p in packs if p['id']==pack['id'])['applied_enabled']
        print('PASS Bedrock addon upload/install/enable, game readiness and disable',flush=True)
    finally:
        sql("UPDATE accounts SET role='customer',disabled=true WHERE id='"+actor+"'")
        sql("DELETE FROM account_sessions WHERE account_id='"+actor+"'")
        print('PASS temporary panel test account revoked',flush=True)

if __name__=='__main__':main()
