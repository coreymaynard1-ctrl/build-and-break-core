#!/usr/bin/env python3
"""Local sandbox invoice -> simulated payment -> real hosting provisioning.
No payment provider APIs and no money collected. Leaves a stopped owned test
service for dashboard review; the existing development container is untouched.
"""
import json, uuid
from lifecycle_e2e import request, wait_service, NODE

def main():
    suffix=str(uuid.uuid4())[:8]
    body={'plan_id':'dev-alpine','node_id':NODE,'name':'Billing E2E '+suffix,'email':'local-test@example.com'}
    key='billing-e2e-'+suffix
    order=request('/billing/orders','POST',body,201,headers={'Idempotency-Key':key})
    replay=request('/billing/orders','POST',body,200,headers={'Idempotency-Key':key})
    assert order['invoice_id']==replay['invoice_id']
    request('/billing/orders','POST',{**body,'name':'different'},409,headers={'Idempotency-Key':key})
    print('PASS unpaid invoice and idempotent order creation',flush=True)
    paid=request('/billing/invoices/'+order['invoice_id']+'/pay-sandbox','POST')
    again=request('/billing/invoices/'+order['invoice_id']+'/pay-sandbox','POST')
    assert paid['service_id']==again['service_id']
    service=wait_service(paid['service_id'],'running')
    invoices=request('/billing/invoices')
    invoice=next(i for i in invoices if i['id']==order['invoice_id'])
    assert invoice['status']=='paid' and invoice['mode']=='sandbox'
    print('PASS simulated payment -> one server/service/job -> real running container',flush=True)
    request('/services/'+service['id']+'/stop','POST',expected=202)
    wait_service(service['id'],'stopped')
    print('PASS billed service stopped through real lifecycle queue',flush=True)
    print(json.dumps({'invoice_id':order['invoice_id'],'server_id':paid['server_id'],'service_id':service['id']}),flush=True)
if __name__=='__main__': main()
