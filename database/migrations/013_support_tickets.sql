CREATE TABLE support_tickets (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 account_id UUID NOT NULL REFERENCES accounts(id),
 subject VARCHAR(160) NOT NULL,
 category TEXT NOT NULL CHECK(category IN ('technical','billing','account','other')),
 status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open','answered','closed')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX support_tickets_account ON support_tickets(account_id,updated_at DESC);
CREATE TABLE support_messages (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 ticket_id UUID NOT NULL REFERENCES support_tickets(id),
 account_id UUID NOT NULL REFERENCES accounts(id),
 message TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX support_messages_ticket ON support_messages(ticket_id,created_at);
