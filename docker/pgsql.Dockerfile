ARG PG_MAJOR=17
FROM postgres:$PG_MAJOR

RUN apt-get update && \
    apt-mark hold locales && \
    apt-get install -y --no-install-recommends \
        postgresql-$PG_MAJOR-postgis-3 \
        postgresql-$PG_MAJOR-pgrouting \
        postgresql-$PG_MAJOR-pgvector && \
    apt-get autoremove -y && \
    apt-mark unhold locales && \
    rm -rf /var/lib/apt/lists/*