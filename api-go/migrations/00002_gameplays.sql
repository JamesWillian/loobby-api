-- Loobby 2.0, Fase 1: as gameplays. Tabelas novas, ao lado das da v1.
--
-- users, groups, group_members, games e device_tokens sao compartilhadas; as
-- tabelas da v1 (events, event_rsvps, ...) nao sao tocadas, para os apps 1.3 e
-- 1.3.1 seguirem funcionando. Nada e migrado nem apagado.
--
-- Por enquanto tudo e privado (a gameplay publica esta pausada): a gameplay do
-- grupo e vista pelos membros, e a unica (sem grupo), por quem entrou pelo
-- convite. Sem coluna de visibilidade: quando a publica voltar, ela entra numa
-- migracao de uma linha, com DEFAULT, sem reescrever a tabela.
--
-- Toda tabela nova tem RLS sem policy (a API de dados do Supabase nao ve nada)
-- e o GRANT ao loobby_backend, que so existe no Supabase -- ver api-go/README.md.

-- +goose Up

-- gameplays: modo VOTING (votacao), FIXED (jogo definido) ou FREE (livre). A
-- votacao fica aberta ate o encerramento: manual (ended_by preenchido) ou
-- automatico 24 h depois do horario marcado (ended_by NULL).
CREATE TABLE public.gameplays (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    group_id uuid,                -- NULL = gameplay unica
    creator_id uuid NOT NULL,
    name text NOT NULL,
    description text,
    mode text NOT NULL,
    platforms jsonb NOT NULL,     -- lista, por causa do crossplay futuro; o servidor exige 1 por enquanto
    scheduled_at timestamp with time zone NOT NULL,
    invite_code text,             -- so na unica
    game_id text,                 -- FIXED: id do catalogo (games.id, o do RAWG); NULL = digitado
    game_name text,               -- FIXED: o nome, sempre
    finalists smallint,           -- VOTING: quantos jogos saem finalistas
    multi_vote boolean,           -- VOTING: cada um vota em varios (um voto por jogo) ou em um so
    voting_round smallint,        -- VOTING: 1 = votacao principal; 2 em diante = desempates
    ended_at timestamp with time zone,
    ended_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT gameplays_pkey PRIMARY KEY (id),
    CONSTRAINT gameplays_invite_code_key UNIQUE (invite_code),
    CONSTRAINT gameplays_group_id_fkey FOREIGN KEY (group_id) REFERENCES public.groups(id) ON DELETE CASCADE,
    CONSTRAINT gameplays_creator_id_fkey FOREIGN KEY (creator_id) REFERENCES public.users(id) ON DELETE RESTRICT,
    CONSTRAINT gameplays_ended_by_fkey FOREIGN KEY (ended_by) REFERENCES public.users(id) ON DELETE SET NULL,
    CONSTRAINT gameplays_mode_check CHECK (mode IN ('VOTING', 'FIXED', 'FREE')),
    CONSTRAINT gameplays_invite_check CHECK ((group_id IS NULL) = (invite_code IS NOT NULL)),
    CONSTRAINT gameplays_platforms_check CHECK (
        jsonb_typeof(platforms) = 'array' AND jsonb_array_length(platforms) >= 1
        AND platforms <@ '["PC", "PLAYSTATION", "XBOX", "SWITCH", "MOBILE"]'::jsonb),
    CONSTRAINT gameplays_fixed_check CHECK (
        CASE WHEN mode = 'FIXED' THEN game_name IS NOT NULL
             ELSE game_id IS NULL AND game_name IS NULL END),
    CONSTRAINT gameplays_voting_check CHECK (
        CASE WHEN mode = 'VOTING' THEN finalists IS NOT NULL AND multi_vote IS NOT NULL AND voting_round IS NOT NULL
             ELSE finalists IS NULL AND multi_vote IS NULL AND voting_round IS NULL END),
    CONSTRAINT gameplays_finalists_check CHECK (finalists BETWEEN 1 AND 20),
    CONSTRAINT gameplays_voting_round_check CHECK (voting_round >= 1),
    CONSTRAINT gameplays_ended_check CHECK (ended_by IS NULL OR ended_at IS NOT NULL)
);

CREATE INDEX gameplays_group_idx ON public.gameplays USING btree (group_id, scheduled_at) WHERE group_id IS NOT NULL;
CREATE INDEX gameplays_active_idx ON public.gameplays USING btree (scheduled_at) WHERE ended_at IS NULL;
CREATE INDEX gameplays_creator_idx ON public.gameplays USING btree (creator_id);

-- Vou / Nao vou / Pendente. Na gameplay do grupo, membro sem linha esta
-- Pendente; na unica, a linha e a entrada pelo convite.
CREATE TABLE public.gameplay_participants (
    gameplay_id uuid NOT NULL,
    user_id uuid NOT NULL,
    status text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT gameplay_participants_pkey PRIMARY KEY (gameplay_id, user_id),
    CONSTRAINT gameplay_participants_gameplay_id_fkey FOREIGN KEY (gameplay_id) REFERENCES public.gameplays(id) ON DELETE CASCADE,
    CONSTRAINT gameplay_participants_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE,
    CONSTRAINT gameplay_participants_status_check CHECK (status IN ('GOING', 'NOT_GOING', 'PENDING'))
);

CREATE INDEX gameplay_participants_user_idx ON public.gameplay_participants USING btree (user_id);

-- Os jogos em votacao: ate 20 por gameplay (regra do servidor), sem repetir o
-- jogo do catalogo nem o nome. Jogo digitado (game_id NULL) vale, sem capa; a
-- capa do jogo do catalogo vem de games.background_image.
--
-- last_round: o jogo e candidato ate essa rodada. O desempate leva os
-- empatados para a rodada seguinte; os demais param na anterior.
CREATE TABLE public.gameplay_games (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    gameplay_id uuid NOT NULL,
    game_id text,
    name text NOT NULL,
    added_by uuid,
    last_round smallint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT gameplay_games_pkey PRIMARY KEY (id),
    CONSTRAINT gameplay_games_id_gameplay_key UNIQUE (id, gameplay_id), -- alvo da FK composta dos votos
    CONSTRAINT gameplay_games_gameplay_id_fkey FOREIGN KEY (gameplay_id) REFERENCES public.gameplays(id) ON DELETE CASCADE,
    CONSTRAINT gameplay_games_added_by_fkey FOREIGN KEY (added_by) REFERENCES public.users(id) ON DELETE SET NULL,
    CONSTRAINT gameplay_games_last_round_check CHECK (last_round >= 1)
);

CREATE UNIQUE INDEX gameplay_games_catalog_key ON public.gameplay_games USING btree (gameplay_id, game_id) WHERE game_id IS NOT NULL;
CREATE UNIQUE INDEX gameplay_games_name_key ON public.gameplay_games USING btree (gameplay_id, lower(name));

-- Um voto por (jogo, rodada, pessoa). Voto unico ou multiplo e regra do
-- servidor. A FK composta garante que o jogo votado e da mesma gameplay.
CREATE TABLE public.gameplay_votes (
    gameplay_game_id uuid NOT NULL,
    voting_round smallint NOT NULL,
    user_id uuid NOT NULL,
    gameplay_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT gameplay_votes_pkey PRIMARY KEY (gameplay_game_id, voting_round, user_id),
    CONSTRAINT gameplay_votes_game_fkey FOREIGN KEY (gameplay_game_id, gameplay_id)
        REFERENCES public.gameplay_games(id, gameplay_id) ON DELETE CASCADE,
    CONSTRAINT gameplay_votes_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE,
    CONSTRAINT gameplay_votes_round_check CHECK (voting_round >= 1)
);

CREATE INDEX gameplay_votes_gameplay_idx ON public.gameplay_votes USING btree (gameplay_id, voting_round, user_id);
CREATE INDEX gameplay_votes_user_idx ON public.gameplay_votes USING btree (user_id);

CREATE TABLE public.gameplay_comments (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    gameplay_id uuid NOT NULL,
    user_id uuid NOT NULL,
    body text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT gameplay_comments_pkey PRIMARY KEY (id),
    CONSTRAINT gameplay_comments_gameplay_id_fkey FOREIGN KEY (gameplay_id) REFERENCES public.gameplays(id) ON DELETE CASCADE,
    CONSTRAINT gameplay_comments_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE
);

CREATE INDEX gameplay_comments_gameplay_idx ON public.gameplay_comments USING btree (gameplay_id, created_at DESC, id DESC);
CREATE INDEX gameplay_comments_user_idx ON public.gameplay_comments USING btree (user_id);

-- Denuncias feitas no app. Ficam aqui para quem administra o Loobby ler; somem
-- com o comentario, quando o autor ou um moderador o apaga.
CREATE TABLE public.gameplay_comment_reports (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    comment_id uuid NOT NULL,
    reporter_id uuid NOT NULL,
    reason text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT gameplay_comment_reports_pkey PRIMARY KEY (id),
    CONSTRAINT gameplay_comment_reports_unique UNIQUE (comment_id, reporter_id),
    CONSTRAINT gameplay_comment_reports_comment_id_fkey FOREIGN KEY (comment_id) REFERENCES public.gameplay_comments(id) ON DELETE CASCADE,
    CONSTRAINT gameplay_comment_reports_reporter_id_fkey FOREIGN KEY (reporter_id) REFERENCES public.users(id) ON DELETE CASCADE
);

CREATE INDEX gameplay_comment_reports_reporter_idx ON public.gameplay_comment_reports USING btree (reporter_id);

ALTER TABLE public.gameplays ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.gameplay_participants ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.gameplay_games ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.gameplay_votes ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.gameplay_comments ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.gameplay_comment_reports ENABLE ROW LEVEL SECURITY;

-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'loobby_backend') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON
      public.gameplays, public.gameplay_participants, public.gameplay_games,
      public.gameplay_votes, public.gameplay_comments, public.gameplay_comment_reports
    TO loobby_backend;
  END IF;
END $$;
-- +goose StatementEnd

-- +goose Down

DROP TABLE public.gameplay_comment_reports;
DROP TABLE public.gameplay_comments;
DROP TABLE public.gameplay_votes;
DROP TABLE public.gameplay_games;
DROP TABLE public.gameplay_participants;
DROP TABLE public.gameplays;
