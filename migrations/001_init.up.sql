create table if not exists teams (
                                     team_name text primary key
);

create table if not exists users (
                                     user_id   text primary key,
                                     username  text not null,
                                     team_name text not null references teams(team_name) on delete restrict,
    is_active boolean not null default true
    );

do $$ begin
  if not exists (select 1 from pg_type where typname = 'pr_status') then
create type pr_status as enum ('OPEN','MERGED');
end if;
end $$;

create table if not exists pull_requests (
                                             pr_id      text primary key,
                                             pr_name    text not null,
                                             author_id  text not null references users(user_id) on delete restrict,
    status     pr_status not null default 'OPEN',
    created_at timestamptz not null default now(),
    merged_at  timestamptz
    );

create table if not exists pr_reviewers (
                                            pr_id   text references pull_requests(pr_id) on delete cascade,
    user_id text references users(user_id) on delete restrict,
    primary key (pr_id, user_id)
    );

create index if not exists idx_users_team on users(team_name);
create index if not exists idx_pr_author on pull_requests(author_id);
create index if not exists idx_pr_reviewers_user on pr_reviewers(user_id);
