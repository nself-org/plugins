ALTER TABLE node ADD COLUMN age_recipient TEXT;
UPDATE node SET age_recipient = (
 SELECT json_extract(doc_json, '$.identity.age_recipient.value')
 FROM capability_snapshot WHERE node_id = node.id AND json_valid(doc_json)
);
