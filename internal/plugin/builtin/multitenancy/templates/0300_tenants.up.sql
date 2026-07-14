CREATE TABLE IF NOT EXISTS public.tenants (
    id TEXT PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE,
    schema_name TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
