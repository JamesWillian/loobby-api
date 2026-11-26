-- Varias contas excluidas (contract/ACHADOS.md §23).
--
-- A exclusao de conta deixa o usuario com username '' (users.Service.DeleteAccount),
-- e a restricao UNIQUE (username) so cabia UMA conta assim: da segunda exclusao
-- em diante, 500, e a conta continuava existindo. O indice unico parcial segue
-- barrando nome repetido, menos o vazio.
--
-- O '' fica, em vez de NULL: os apps 1.3 e 1.3.1 declaram username nao anulavel
-- e reconhecem a conta excluida pelo nome em branco, como o modulo teams.
--
-- O indice herda o nome da restricao, e o erro de nome repetido continua o
-- mesmo. Nao depende de codigo novo: o binario de antes ja passa a excluir.

-- +goose Up

ALTER TABLE public.users DROP CONSTRAINT users_username_key;
CREATE UNIQUE INDEX users_username_key ON public.users USING btree (username) WHERE username <> '';

-- +goose Down

-- Falha se ja houver duas contas excluidas: a restricao antiga nao cabe nelas.
DROP INDEX public.users_username_key;
ALTER TABLE public.users ADD CONSTRAINT users_username_key UNIQUE (username);
