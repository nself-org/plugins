-- plugin-llm-gateway: rollback init migration
DROP TABLE IF EXISTS public.np_llm_gateway_quota_usage;
DROP TABLE IF EXISTS public.np_llm_gateway_requests;
