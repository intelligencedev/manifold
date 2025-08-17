package databases

import "github.com/jackc/pgx/v5/pgxpool"

// Close allows pg-backed structs to be closed via Manager.Close reflection helper.
func (p *pgSearch) Close() { p.pool.Close() }
func (p *pgVector) Close() { p.pool.Close() }
func (p *pgGraph) Close()  { p.pool.Close() }

// Ensure pgxpool is referenced where needed to avoid unused import pruning
var _ *pgxpool.Pool
