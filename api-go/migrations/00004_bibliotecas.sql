-- Loobby 2.0, Fase 3: as bibliotecas de jogos (internal/library).
--
-- user_library e a biblioteca da pessoa: os jogos dela, com o "tenho o jogo"
-- em owned_platforms. So a propria pessoa le a lista. Os outros veem apenas o
-- match, nas gameplays que dividem com ela: quem vai e tem o jogo.
--
-- group_library sao os jogos que o grupo joga, para montar a votacao depressa.
--
-- As duas so aceitam jogo do catalogo (a FK para games, o cache do RAWG). As
-- plataformas em que o jogo existe nao sao guardadas: saem de games.platforms
-- na leitura (internal/platform).
--
-- Como na 00002: RLS sem policy e o GRANT ao loobby_backend, que so existe no
-- Supabase -- ver api-go/README.md.

-- +goose Up

CREATE TABLE public.user_library (
    user_id uuid NOT NULL,
    game_id text NOT NULL,
    owned_platforms jsonb DEFAULT '[]'::jsonb NOT NULL, -- o "tenho o jogo"; [] = so na biblioteca
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT user_library_pkey PRIMARY KEY (user_id, game_id), -- serve tambem o match
    CONSTRAINT user_library_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE,
    CONSTRAINT user_library_game_id_fkey FOREIGN KEY (game_id) REFERENCES public.games(id) ON DELETE CASCADE,
    CONSTRAINT user_library_owned_platforms_check CHECK (
        jsonb_typeof(owned_platforms) = 'array'
        AND owned_platforms <@ '["PC", "PLAYSTATION", "XBOX", "SWITCH", "MOBILE"]'::jsonb)
);

CREATE TABLE public.group_library (
    group_id uuid NOT NULL,
    game_id text NOT NULL,
    added_by uuid, -- NULL: a conta de quem adicionou foi excluida
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT group_library_pkey PRIMARY KEY (group_id, game_id),
    CONSTRAINT group_library_group_id_fkey FOREIGN KEY (group_id) REFERENCES public.groups(id) ON DELETE CASCADE,
    CONSTRAINT group_library_game_id_fkey FOREIGN KEY (game_id) REFERENCES public.games(id) ON DELETE CASCADE,
    CONSTRAINT group_library_added_by_fkey FOREIGN KEY (added_by) REFERENCES public.users(id) ON DELETE SET NULL
);

ALTER TABLE public.user_library ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.group_library ENABLE ROW LEVEL SECURITY;

-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'loobby_backend') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON public.user_library, public.group_library TO loobby_backend;
  END IF;
END $$;
-- +goose StatementEnd

-- +goose Down

DROP TABLE public.group_library;
DROP TABLE public.user_library;
