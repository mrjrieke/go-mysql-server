// Copyright 2021 Dolthub, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package plan

import (
	"fmt"
	"github.com/dolthub/go-mysql-server/sql/expression"
	"gopkg.in/src-d/go-errors.v1"

	"github.com/dolthub/go-mysql-server/sql"
)

var (
	// ErrNoCheckConstraintSupport is returned when the table does not support CONSTRAINT CHECK operations.
	ErrNoCheckConstraintSupport = errors.NewKind("the table does not support check constraint operations: %s")
)

type CreateCheck struct {
	UnaryNode
	ChDef *sql.CheckConstraint
}

type DropCheck struct {
	UnaryNode
	ChDef *sql.CheckConstraint
}

func NewAlterAddCheck(table sql.Node, chDef *sql.CheckConstraint) *CreateCheck {
	return &CreateCheck{
		UnaryNode: UnaryNode{table},
		ChDef:     chDef,
	}
}

func NewAlterDropCheck(table sql.Node, chDef *sql.CheckConstraint) *DropCheck {
	return &DropCheck{
		UnaryNode: UnaryNode{Child: table},
		ChDef:     chDef,
	}
}

func getCheckAlterable(node sql.Node) (sql.CheckAlterableTable, error) {
	switch node := node.(type) {
	case sql.CheckAlterableTable:
		return node, nil
	case *ResolvedTable:
		return getCheckAlterableTable(node.Table)
	default:
		return nil, ErrNoCheckConstraintSupport.New(node.String())
	}
}

func getCheckAlterableTable(t sql.Table) (sql.CheckAlterableTable, error) {
	switch t := t.(type) {
	case sql.CheckAlterableTable:
		return t, nil
	case sql.TableWrapper:
		return getCheckAlterableTable(t.Underlying())
	default:
		return nil, ErrNoCheckConstraintSupport.New(t.Name())
	}
}

// Execute inserts the rows in the database.
func (p *CreateCheck) Execute(ctx *sql.Context) error {
	chAlterable, err := getCheckAlterable(p.UnaryNode.Child)
	if err != nil {
		return err
	}

	// Make sure that all columns are valid, in the table, and there are no duplicates
	cols := make(map[string]bool)
	for _, col := range chAlterable.Schema() {
		cols[col.Name] = true
	}

	sql.Inspect(p.ChDef.Expr, func(expr sql.Expression) bool {
		switch expr := expr.(type) {
		case *expression.UnresolvedColumn:
			if _, ok := cols[expr.Name()]; !ok {
				err = sql.ErrTableColumnNotFound.New(expr.Name())
				return false
			}
		case *expression.UnresolvedFunction:
			err = sql.ErrInvalidConstraintFunctionsNotSupported.New(expr.String())
			return false
		case *Subquery:
			err = sql.ErrInvalidConstraintSubqueryNotSupported.New(expr.String())
			return false
		}
		return true
	})
	if err != nil {
		return err
	}
	//switch p.ChDef.Expr.(type):
	//	case expression.BinaryExpression:
	//for _, chCol := range p.ChDef.Expr. {
	//	if seen, ok := seenCols[fkCol]; ok {
	//		if !seen {
	//			seenCols[fkCol] = true
	//		} else {
	//			return ErrAddForeignKeyDuplicateColumn.New(fkCol)
	//		}
	//	} else {
	//		return sql.ErrTableColumnNotFound.New(fkCol)
	//	}
	//}

	return chAlterable.CreateCheckConstraint(ctx, p.ChDef.Name, p.ChDef.Expr, p.ChDef.Enforced)
}

// Execute inserts the rows in the database.
func (p *DropCheck) Execute(ctx *sql.Context) error {
	chAlterable, err := getCheckAlterable(p.UnaryNode.Child)
	if err != nil {
		return err
	}
	return chAlterable.DropCheckConstraint(ctx, p.ChDef.Name)
}

// RowIter implements the Node interface.
func (p *DropCheck) RowIter(ctx *sql.Context, row sql.Row) (sql.RowIter, error) {
	err := p.Execute(ctx)
	if err != nil {
		return nil, err
	}
	return sql.RowsToRowIter(), nil
}

// WithChildren implements the Node interface.
func (p *DropCheck) WithChildren(children ...sql.Node) (sql.Node, error) {
	if len(children) != 1 {
		return nil, sql.ErrInvalidChildrenNumber.New(p, len(children), 1)
	}
	return NewAlterDropCheck(children[0], p.ChDef), nil
}

// WithChildren implements the Node interface.
func (p *CreateCheck) WithChildren(children ...sql.Node) (sql.Node, error) {
	if len(children) != 1 {
		return nil, sql.ErrInvalidChildrenNumber.New(p, len(children), 1)
	}
	return NewAlterAddCheck(children[0], p.ChDef), nil
}

func (p *CreateCheck) Schema() sql.Schema { return nil }
func (p *DropCheck) Schema() sql.Schema   { return nil }

func (p *CreateCheck) RowIter(ctx *sql.Context, row sql.Row) (sql.RowIter, error) {
	err := p.Execute(ctx)
	if err != nil {
		return nil, err
	}
	return sql.RowsToRowIter(), nil
}

func (p DropCheck) String() string {
	pr := sql.NewTreePrinter()
	_ = pr.WriteNode("DropCheck(%s)", p.ChDef.Name)
	_ = pr.WriteChildren(fmt.Sprintf("Table(%s)", p.UnaryNode.Child.String()))
	return pr.String()
}

func (p CreateCheck) String() string {
	pr := sql.NewTreePrinter()
	_ = pr.WriteNode("AddCheck(%s)", p.ChDef.Name)
	_ = pr.WriteChildren(fmt.Sprintf("Table(%s)", p.UnaryNode.Child.String()))
	return pr.String()
}
