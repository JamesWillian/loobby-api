-- Seed do ambiente local de comparacao (contract/local/oracle.sh). Espelha o que
-- a suite de contrato encontra em producao: as duas contas de teste, com email
-- verificado, e o grupo que a primeira ja tinha. A senha das duas e
-- "senha-local-123" -- so existe aqui.
--
-- O dono do grupo nao e inserido em group_members: o gatilho do banco faz isso
-- (contract/ACHADOS.md §9).
INSERT INTO users(id,username,displayname,avatar_url,created_at,auth_provider) VALUES
 ('54f29b67-cb8b-4435-8b72-43c17f2f4b8f','user_demo','Contract Baseline',NULL,'2026-04-27T18:03:35.736666Z',1),
 ('61d1ba14-2a43-4e31-a6b8-2ed31cadbcdf','user_2e6331','user_2e6331',NULL,'2026-05-01T12:00:00.123456Z',1);
INSERT INTO user_credentials(user_id,password_hash,roles,email,email_verified,created_at) VALUES
 ('54f29b67-cb8b-4435-8b72-43c17f2f4b8f','$2a$10$sl3ZCrphczJTtp1Yf3MLJeC/y/Lc3EGgTjsxwEHD1ByCJZAvPNs36','USER','user@loobby.app',true,'2026-04-27T18:03:35.736666Z'),
 ('61d1ba14-2a43-4e31-a6b8-2ed31cadbcdf','$2a$10$sl3ZCrphczJTtp1Yf3MLJeC/y/Lc3EGgTjsxwEHD1ByCJZAvPNs36','USER','contract-suite@loobby.app',true,'2026-05-01T12:00:00.123456Z');
INSERT INTO groups(id,name,invite_code,owner_id,created_at) VALUES
 ('11111111-2222-4333-8444-555555555555','My First Group','$-2ISBVV','54f29b67-cb8b-4435-8b72-43c17f2f4b8f','2026-04-27T18:10:00Z');
