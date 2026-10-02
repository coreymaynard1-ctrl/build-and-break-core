#!/usr/bin/env python3
"""Real Minecraft Java provisioning + game-protocol status + lifecycle smoke test.
May download the free Docker image/server files. Uses a test service and explicitly
accepts the Minecraft EULA for that test. Leaves the new service stopped for review.
"""
import os, json, socket, struct, time, uuid
from lifecycle_e2e import request, wait_service, NODE
PORT=25575

def varint(n):
    out=bytearray()
    while True:
        part=n & 127;n >>= 7;out.append(part | (128 if n else 0))
        if not n:return bytes(out)
def recv_exact(s,n):
    out=b''
    while len(out)<n:
        chunk=s.recv(n-len(out))
        if not chunk:raise ConnectionError('unexpected EOF')
        out+=chunk
    return out
def read_varint(s):
    value=0
    for i in range(5):
        b=recv_exact(s,1)[0];value |= (b&127)<<(7*i)
        if not b&128:return value
    raise ValueError('invalid varint')
def ping():
    with socket.create_connection(('127.0.0.1',PORT),timeout=3) as s:
        host=b'localhost';packet=b'\x00'+varint(767)+varint(len(host))+host+struct.pack('>H',PORT)+b'\x01'
        s.sendall(varint(len(packet))+packet+b'\x01\x00')
        length=read_varint(s)
        if length>1024*1024:raise ValueError('oversized status response')
        assert read_varint(s)==0
        text_len=read_varint(s)
        if text_len>1024*1024:raise ValueError('oversized status JSON')
        return json.loads(recv_exact(s,text_len))
def ready(timeout=240):
    end=time.monotonic()+timeout
    while time.monotonic()<end:
        try:
            status=ping();assert status['version']['name']=='1.21.1';return status
        except (OSError,AssertionError,ValueError,ConnectionError):time.sleep(3)
    raise AssertionError('Minecraft did not answer a game-protocol status request')
def main():
    if os.environ.get('BNB_MINECRAFT_RESUME')=='1':
        with open('/tmp/bnb-minecraft-e2e.json') as f: saved=json.load(f)
        server={'id':saved['server_id']};sid=saved['service_id']
        request('/services/'+sid+'/retry','POST',expected=202)
        print('Retrying existing owned Minecraft service through Core',flush=True)
    else:
        suffix=str(uuid.uuid4())[:8]
        server=request('/servers','POST',{'node_id':NODE,'name':'Minecraft E2E '+suffix,'memory_mb':2048,'disk_mb':4096,'cpu_threads':2},201)
        body={'server_id':server['id'],'name':'Minecraft E2E '+suffix,'template':'minecraft-java','memory_mb':2048,'cpu_threads':2,'port':PORT,'accept_eula':True}
        service=request('/services','POST',body,202);sid=service['id']
        with open('/tmp/bnb-minecraft-e2e.json','w') as f:json.dump({'server_id':server['id'],'service_id':sid,'port':PORT},f)
    current=wait_service(sid,'running',timeout=660)
    print('PASS Minecraft create: runtime identity '+current['runtime_id'],flush=True)
    state=ready();print('PASS actual Minecraft 1.21.1 protocol status: '+str(state.get('description','')),flush=True)
    for action,status in [('stop','stopped'),('start','running'),('restart','running')]:
        request('/services/'+sid+'/'+action,'POST',expected=202);wait_service(sid,status,timeout=120)
        if status=='running':ready()
        print('PASS Minecraft '+action,flush=True)
    request('/services/'+sid+'/stop','POST',expected=202);wait_service(sid,'stopped',timeout=120)
    print('PASS Minecraft left stopped with named world volume retained',flush=True)
    print(json.dumps({'service_id':sid,'server_id':server['id'],'port':PORT}),flush=True)
if __name__=='__main__':main()
