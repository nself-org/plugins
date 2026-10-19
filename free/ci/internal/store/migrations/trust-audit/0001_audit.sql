CREATE INDEX trust_audit_unsealed ON audit(seq) WHERE head IS NULL;
