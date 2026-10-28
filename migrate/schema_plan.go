package migrate

import (
	"context"
	"fmt"
	"strings"
)

// SchemaPlan stores DDL and DML statements for ordered execution.
type SchemaPlan struct {
	ctx        *MigrationContext
	statements []SQLStatement
	err        error
}

func (p *SchemaPlan) add(query string, err error, args ...any) *SchemaPlan {
	if p.err != nil {
		return p
	}
	if err != nil {
		p.err = err
		return p
	}
	p.statements = append(p.statements, SQLStatement{Query: query, Args: append([]any(nil), args...)})
	return p
}

func (p *SchemaPlan) addStatement(stmt SQLStatement, err error) *SchemaPlan {
	if p.err != nil {
		return p
	}
	if err != nil {
		p.err = err
		return p
	}
	p.statements = append(p.statements, SQLStatement{Query: stmt.Query, Args: append([]any(nil), stmt.Args...)})
	return p
}

// existenceGuard is the single guard shared by every *IfExists plan variant.
// In dry-run mode emit runs unconditionally so the statement stays visible for
// inspection. Otherwise probe reports whether the guarded object exists: a
// probe failure is recorded on the plan, and emit runs only when the existence
// result matches emitOnExists — true for drop variants (emit when the object
// exists), false for create/add variants (emit when it is absent).
func (p *SchemaPlan) existenceGuard(ctx context.Context, emitOnExists bool, probe func(context.Context) (bool, error), emit func() *SchemaPlan) *SchemaPlan {
	if p.ctx.dryRun {
		return emit()
	}
	exists, err := probe(ctx)
	if err != nil {
		p.err = err
		return p
	}
	if exists != emitOnExists {
		return p
	}
	return emit()
}

// Raw appends a raw SQL statement.
func (p *SchemaPlan) Raw(sql string, args ...any) *SchemaPlan {
	return p.add(sql, nil, args...)
}

// CreateTable appends a CREATE TABLE statement.
func (p *SchemaPlan) CreateTable(table string, columns *ColumnList, options ...string) *SchemaPlan {
	query, err := p.ctx.dialect.CreateTable(table, columns, strings.Join(options, " "))
	return p.add(query, err)
}

// CreateTableIfNotExists appends CREATE TABLE when the table is absent.
func (p *SchemaPlan) CreateTableIfNotExists(ctx context.Context, table string, columns *ColumnList, options ...string) *SchemaPlan {
	return p.existenceGuard(ctx, false,
		func(c context.Context) (bool, error) { return p.ctx.TableExists(c, table) },
		func() *SchemaPlan { return p.CreateTable(table, columns, options...) })
}

// DropTable appends a DROP TABLE statement.
func (p *SchemaPlan) DropTable(table string) *SchemaPlan {
	query, err := p.ctx.dialect.DropTable(table)
	return p.add(query, err)
}

// DropTableIfExists appends DROP TABLE when the table exists.
func (p *SchemaPlan) DropTableIfExists(ctx context.Context, table string) *SchemaPlan {
	return p.existenceGuard(ctx, true,
		func(c context.Context) (bool, error) { return p.ctx.TableExists(c, table) },
		func() *SchemaPlan { return p.DropTable(table) })
}

// RenameTable appends a table rename statement.
func (p *SchemaPlan) RenameTable(oldName, newName string) *SchemaPlan {
	query, err := p.ctx.dialect.RenameTable(oldName, newName)
	return p.add(query, err)
}

// TruncateTable appends a table truncate statement.
func (p *SchemaPlan) TruncateTable(table string) *SchemaPlan {
	query, err := p.ctx.dialect.TruncateTable(table)
	return p.add(query, err)
}

// AddColumn appends an ADD COLUMN statement.
func (p *SchemaPlan) AddColumn(table, column string, builder *ColumnBuilder) *SchemaPlan {
	query, err := p.ctx.dialect.AddColumn(table, column, builder)
	return p.add(query, err)
}

// AddColumnIfNotExists appends ADD COLUMN when the column is absent.
func (p *SchemaPlan) AddColumnIfNotExists(ctx context.Context, table, column string, builder *ColumnBuilder) *SchemaPlan {
	return p.existenceGuard(ctx, false,
		func(c context.Context) (bool, error) { return p.ctx.ColumnExists(c, table, column) },
		func() *SchemaPlan { return p.AddColumn(table, column, builder) })
}

// AlterColumn appends an ALTER COLUMN statement.
func (p *SchemaPlan) AlterColumn(table, column string, builder *ColumnBuilder) *SchemaPlan {
	query, err := p.ctx.dialect.AlterColumn(table, column, builder)
	return p.add(query, err)
}

// DropColumn appends a DROP COLUMN statement.
func (p *SchemaPlan) DropColumn(table, column string) *SchemaPlan {
	query, err := p.ctx.dialect.DropColumn(table, column)
	return p.add(query, err)
}

// DropColumnIfExists appends DROP COLUMN when the column exists.
func (p *SchemaPlan) DropColumnIfExists(ctx context.Context, table, column string) *SchemaPlan {
	return p.existenceGuard(ctx, true,
		func(c context.Context) (bool, error) { return p.ctx.ColumnExists(c, table, column) },
		func() *SchemaPlan { return p.DropColumn(table, column) })
}

// RenameColumn appends a column rename statement.
func (p *SchemaPlan) RenameColumn(table, oldName, newName string) *SchemaPlan {
	query, err := p.ctx.dialect.RenameColumn(table, oldName, newName)
	return p.add(query, err)
}

// CreateIndex appends a CREATE INDEX statement.
func (p *SchemaPlan) CreateIndex(name, table string, columns []string, unique bool) *SchemaPlan {
	query, err := p.ctx.dialect.CreateIndex(name, table, columns, unique)
	return p.add(query, err)
}

// CreateIndexIfNotExists appends CREATE INDEX when the index is absent.
func (p *SchemaPlan) CreateIndexIfNotExists(ctx context.Context, name, table string, columns []string, unique bool) *SchemaPlan {
	return p.existenceGuard(ctx, false,
		func(c context.Context) (bool, error) { return p.ctx.IndexExists(c, table, name) },
		func() *SchemaPlan { return p.CreateIndex(name, table, columns, unique) })
}

// DropIndex appends a DROP INDEX statement.
func (p *SchemaPlan) DropIndex(name, table string) *SchemaPlan {
	query, err := p.ctx.dialect.DropIndex(name, table)
	return p.add(query, err)
}

// DropIndexIfExists appends DROP INDEX when the index exists.
func (p *SchemaPlan) DropIndexIfExists(ctx context.Context, name, table string) *SchemaPlan {
	return p.existenceGuard(ctx, true,
		func(c context.Context) (bool, error) { return p.ctx.IndexExists(c, table, name) },
		func() *SchemaPlan { return p.DropIndex(name, table) })
}

// AddPrimaryKey appends an ADD PRIMARY KEY statement.
func (p *SchemaPlan) AddPrimaryKey(name, table string, columns []string) *SchemaPlan {
	query, err := p.ctx.dialect.AddPrimaryKey(name, table, columns)
	return p.add(query, err)
}

// AddPrimaryKeyIfNotExists appends ADD PRIMARY KEY when the constraint is absent.
func (p *SchemaPlan) AddPrimaryKeyIfNotExists(ctx context.Context, name, table string, columns []string) *SchemaPlan {
	return p.existenceGuard(ctx, false,
		func(c context.Context) (bool, error) { return p.ctx.ConstraintExists(c, table, name) },
		func() *SchemaPlan { return p.AddPrimaryKey(name, table, columns) })
}

// DropPrimaryKey appends a DROP PRIMARY KEY statement.
func (p *SchemaPlan) DropPrimaryKey(name, table string) *SchemaPlan {
	query, err := p.ctx.dialect.DropPrimaryKey(name, table)
	return p.add(query, err)
}

// DropPrimaryKeyIfExists appends DROP PRIMARY KEY when the constraint exists.
func (p *SchemaPlan) DropPrimaryKeyIfExists(ctx context.Context, name, table string) *SchemaPlan {
	return p.existenceGuard(ctx, true,
		func(c context.Context) (bool, error) { return p.ctx.ConstraintExists(c, table, name) },
		func() *SchemaPlan { return p.DropPrimaryKey(name, table) })
}

// AddForeignKey appends an ADD FOREIGN KEY statement.
func (p *SchemaPlan) AddForeignKey(name, table string, columns []string, refTable string, refColumns []string, onDelete ForeignKeyAction, onUpdate ForeignKeyAction) *SchemaPlan {
	query, err := p.ctx.dialect.AddForeignKey(name, table, columns, refTable, refColumns, onDelete, onUpdate)
	return p.add(query, err)
}

// AddForeignKeyIfNotExists appends ADD FOREIGN KEY when the constraint is absent.
func (p *SchemaPlan) AddForeignKeyIfNotExists(ctx context.Context, name, table string, columns []string, refTable string, refColumns []string, onDelete ForeignKeyAction, onUpdate ForeignKeyAction) *SchemaPlan {
	return p.existenceGuard(ctx, false,
		func(c context.Context) (bool, error) { return p.ctx.ForeignKeyExists(c, table, name) },
		func() *SchemaPlan {
			return p.AddForeignKey(name, table, columns, refTable, refColumns, onDelete, onUpdate)
		})
}

// DropForeignKey appends a DROP FOREIGN KEY statement.
func (p *SchemaPlan) DropForeignKey(name, table string) *SchemaPlan {
	query, err := p.ctx.dialect.DropForeignKey(name, table)
	return p.add(query, err)
}

// DropForeignKeyIfExists appends DROP FOREIGN KEY when the constraint exists.
func (p *SchemaPlan) DropForeignKeyIfExists(ctx context.Context, name, table string) *SchemaPlan {
	return p.existenceGuard(ctx, true,
		func(c context.Context) (bool, error) { return p.ctx.ForeignKeyExists(c, table, name) },
		func() *SchemaPlan { return p.DropForeignKey(name, table) })
}

// AddCommentOnColumn appends a column comment statement.
func (p *SchemaPlan) AddCommentOnColumn(table, column, comment string) *SchemaPlan {
	query, err := p.ctx.dialect.AddCommentOnColumn(table, column, comment)
	return p.add(query, err)
}

// DropCommentFromColumn appends a drop column comment statement.
func (p *SchemaPlan) DropCommentFromColumn(table, column string) *SchemaPlan {
	query, err := p.ctx.dialect.DropCommentFromColumn(table, column)
	return p.add(query, err)
}

// AddCommentOnTable appends a table comment statement.
func (p *SchemaPlan) AddCommentOnTable(table, comment string) *SchemaPlan {
	query, err := p.ctx.dialect.AddCommentOnTable(table, comment)
	return p.add(query, err)
}

// DropCommentFromTable appends a drop table comment statement.
func (p *SchemaPlan) DropCommentFromTable(table string) *SchemaPlan {
	query, err := p.ctx.dialect.DropCommentFromTable(table)
	return p.add(query, err)
}

// Insert appends an INSERT statement.
func (p *SchemaPlan) Insert(table string, row Row) *SchemaPlan {
	return p.addStatement(p.ctx.dialect.Insert(table, row))
}

// BatchInsert appends a multi-row INSERT statement.
func (p *SchemaPlan) BatchInsert(table string, columns []string, rows [][]any) *SchemaPlan {
	return p.addStatement(p.ctx.dialect.BatchInsert(table, columns, rows))
}

// Update appends an UPDATE statement.
func (p *SchemaPlan) Update(table string, row Row, condition string, args ...any) *SchemaPlan {
	return p.addStatement(p.ctx.dialect.Update(table, row, condition, args...))
}

// Delete appends a DELETE statement.
func (p *SchemaPlan) Delete(table string, condition string, args ...any) *SchemaPlan {
	return p.addStatement(p.ctx.dialect.Delete(table, condition, args...))
}

// Statements returns a defensive copy of the SQL statements accumulated by
// the plan. It is intended for inspection in tests (for example, asserting
// generated SQL in dry-run mode); production code should normally call Exec.
func (p *SchemaPlan) Statements() []SQLStatement {
	out := make([]SQLStatement, len(p.statements))
	for i, s := range p.statements {
		out[i] = SQLStatement{Query: s.Query, Args: append([]any(nil), s.Args...)}
	}
	return out
}

// Exec executes the plan in order unless dry-run is enabled.
func (p *SchemaPlan) Exec(ctx context.Context) error {
	if p.err != nil {
		return p.err
	}
	if p.ctx == nil || p.ctx.db == nil {
		return fmt.Errorf("migrate: database handle is nil")
	}
	if p.ctx.dryRun {
		return nil
	}
	for _, statement := range p.statements {
		if _, err := p.ctx.db.ExecContext(ctx, statement.Query, statement.Args...); err != nil {
			return err
		}
	}
	return nil
}
