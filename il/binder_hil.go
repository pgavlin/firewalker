package il

import (
	"strings"

	"github.com/hashicorp/hil/ast"
	"github.com/hashicorp/terraform/config"
	"github.com/pkg/errors"
)

// bindArithmetic binds an HIL arithmetic expression.
func (b *propertyBinder) bindArithmetic(n *ast.Arithmetic) (BoundExpr, error) {
	exprs, err := b.bindExprs(n.Exprs)
	if err != nil {
		return nil, err
	}

	return &BoundArithmetic{HILNode: n, Exprs: exprs}, nil
}

// bindCall binds an HIL call expression. This involves binding the call's arguments, then using the name of the called
// function to determine the type of the call expression. The binder curretly only supports a subset of the functions
// supported by terraform.
func (b *propertyBinder) bindCall(n *ast.Call) (BoundExpr, error) {
	args, err := b.bindExprs(n.Args)
	if err != nil {
		return nil, err
	}

	exprType := TypeUnknown
	switch n.Func {
	case "base64decode":
		exprType = TypeString
	case "base64encode":
		exprType = TypeString
	case "chomp":
		exprType = TypeString
	case "element":
		if args[0].Type().IsList() {
			exprType = args[0].Type().ElementType()
		}
	case "file":
		exprType = TypeString
	case "format":
		exprType = TypeString
	case "list":
		exprType = TypeUnknown.ListOf()
	case "lookup":
		// nothing to do
	case "map":
		if len(args)%2 != 0 {
			return nil, errors.Errorf("the numbner of arguments to \"map\" must be even")
		}
		exprType = TypeMap
	case "split":
		exprType = TypeString.ListOf()
	default:
		return nil, errors.Errorf("NYI: call to %s", n.Func)
	}

	return &BoundCall{HILNode: n, ExprType: exprType, Args: args}, nil
}

// bindConditional binds an HIL conditional expression.
func (b *propertyBinder) bindConditional(n *ast.Conditional) (BoundExpr, error) {
	condExpr, err := b.bindExpr(n.CondExpr)
	if err != nil {
		return nil, err
	}
	trueExpr, err := b.bindExpr(n.TrueExpr)
	if err != nil {
		return nil, err
	}
	falseExpr, err := b.bindExpr(n.FalseExpr)
	if err != nil {
		return nil, err
	}

	// If the types of both branches match, then the type of the expression is that of the branches. If the types of
	// both branches differ, then mark the type as unknown.
	exprType := trueExpr.Type()
	if exprType != falseExpr.Type() {
		exprType = TypeUnknown
	}

	return &BoundConditional{
		HILNode:   n,
		ExprType:  exprType,
		CondExpr:  condExpr,
		TrueExpr:  trueExpr,
		FalseExpr: falseExpr,
	}, nil
}

// bindIndex binds an HIL index expression.
func (b *propertyBinder) bindIndex(n *ast.Index) (BoundExpr, error) {
	boundTarget, err := b.bindExpr(n.Target)
	if err != nil {
		return nil, err
	}
	boundKey, err := b.bindExpr(n.Key)
	if err != nil {
		return nil, err
	}

	// If the target type is not a list, then the type of the expression is unknown. Otherwise it is the element type
	// of the list.
	exprType := TypeUnknown
	targetType := boundTarget.Type()
	if targetType.IsList() {
		exprType = targetType.ElementType()
	}

	return &BoundIndex{
		HILNode:    n,
		ExprType:   exprType,
		TargetExpr: boundTarget,
		KeyExpr:    boundKey,
	}, nil
}

// bindLiteral binds an HIL literal expression. The literal must be of type bool, int, float, or string.
func (b *propertyBinder) bindLiteral(n *ast.LiteralNode) (BoundExpr, error) {
	exprType := TypeUnknown
	switch n.Typex {
	case ast.TypeBool:
		exprType = TypeBool
	case ast.TypeInt, ast.TypeFloat:
		exprType = TypeNumber
	case ast.TypeString:
		exprType = TypeString
	default:
		return nil, errors.Errorf("Unexpected literal type %v", n.Typex)
	}

	return &BoundLiteral{ExprType: exprType, Value: n.Value}, nil
}

// bindOutput binds an HIL output expression.
func (b *propertyBinder) bindOutput(n *ast.Output) (BoundExpr, error) {
	exprs, err := b.bindExprs(n.Exprs)
	if err != nil {
		return nil, err
	}

	// Project a single-element output to the element itself.
	if len(exprs) == 1 {
		return exprs[0], nil
	}

	return &BoundOutput{HILNode: n, Exprs: exprs}, nil
}

// bindVariableAccess binds an HIL variable access expression. This involves first interpreting the variable name as a
// Terraform interpolated variable, then using the result of that interpretation to decide which graph node the
// variable access refers to, if any: count, path, and Terraformn variables may not refer to graph nodes. It is an
// error for a variable access to refer to a non-existent node.
func (b *propertyBinder) bindVariableAccess(n *ast.VariableAccess) (BoundExpr, error) {
	tfVar, err := config.NewInterpolatedVariable(n.Name)
	if err != nil {
		return nil, err
	}

	elements, sch, exprType, ilNode := []string(nil), Schemas{}, TypeUnknown, Node(nil)
	switch v := tfVar.(type) {
	case *config.CountVariable:
		// "count."
		if v.Type != config.CountValueIndex {
			return nil, errors.Errorf("unsupported count variable %s", v.FullKey())
		}

		if !b.hasCountIndex {
			return nil, errors.Errorf("no count index in scope")
		}

		exprType = TypeNumber
	case *config.LocalVariable:
		// "local."
		l, ok := b.builder.locals[v.Name]
		if !ok {
			return nil, errors.Errorf("unknown local %v", v.Name)
		}
		ilNode = l

		exprType = TypeUnknown.OutputOf()
	case *config.ModuleVariable:
		// "module."
		m, ok := b.builder.modules[v.Name]
		if !ok {
			return nil, errors.Errorf("unknown module %v", v.Name)
		}
		ilNode = m

		exprType = TypeUnknown.OutputOf()
	case *config.PathVariable:
		// "path."
		return nil, errors.New("NYI: path variables")
	case *config.ResourceVariable:
		// default

		// Look up the resource.
		r, ok := b.builder.resources[v.ResourceId()]
		if !ok {
			return nil, errors.Errorf("unknown resource %v", v.ResourceId())
		}
		ilNode = r

		// Ensure that the resource has a provider.
		if err := b.builder.ensureProvider(r); err != nil {
			return nil, err
		}

		// Fetch the resource's schema info.
		sch = r.Schemas()

		// Parse the path of the accessed field (name{.property}+).
		elements = strings.Split(v.Field, ".")
		elemSch := sch
		for _, e := range elements {
			elemSch = elemSch.PropertySchemas(e)
		}

		// Handle multi-references (splats and indexes).
		exprType = elemSch.Type().OutputOf()
		if v.Multi && v.Index == -1 {
			exprType = exprType.ListOf()
		}
	case *config.SelfVariable:
		// "self."
		return nil, errors.New("NYI: self variables")
	case *config.SimpleVariable:
		// "[^.]\+"
		return nil, errors.New("NYI: simple variables")
	case *config.TerraformVariable:
		// "terraform."
		return nil, errors.New("NYI: terraform variables")
	case *config.UserVariable:
		// "var."
		if v.Elem != "" {
			return nil, errors.New("NYI: user variable elements")
		}

		// Look up the variable.
		vn, ok := b.builder.variables[v.Name]
		if !ok {
			return nil, errors.Errorf("unknown variable %s", v.Name)
		}
		ilNode = vn

		// If the variable does not have a default, its type is string. If it does have a default, its type is string
		// iff the default's type is also string. Note that we don't try all that hard here to get things right, and we
		// more likely than not need to do better.
		exprType = TypeString
		if vn.DefaultValue != nil && vn.DefaultValue.Type() != TypeString {
			exprType = TypeUnknown
		}
	default:
		return nil, errors.Errorf("unexpected variable type %T", v)
	}

	return &BoundVariableAccess{
		HILNode:  n,
		Elements: elements,
		Schemas:  sch,
		ExprType: exprType,
		TFVar:    tfVar,
		ILNode:   ilNode,
	}, nil
}

// bindExprs binds the list of HIL expressions and returns the resulting list.
func (b *propertyBinder) bindExprs(ns []ast.Node) ([]BoundExpr, error) {
	boundExprs := make([]BoundExpr, len(ns))
	for i, n := range ns {
		bn, err := b.bindExpr(n)
		if err != nil {
			return nil, err
		}
		boundExprs[i] = bn
	}
	return boundExprs, nil
}

// bindExpr binds a single HIL expression.
func (b *propertyBinder) bindExpr(n ast.Node) (BoundExpr, error) {
	switch n := n.(type) {
	case *ast.Arithmetic:
		return b.bindArithmetic(n)
	case *ast.Call:
		return b.bindCall(n)
	case *ast.Conditional:
		return b.bindConditional(n)
	case *ast.Index:
		return b.bindIndex(n)
	case *ast.LiteralNode:
		return b.bindLiteral(n)
	case *ast.Output:
		return b.bindOutput(n)
	case *ast.VariableAccess:
		return b.bindVariableAccess(n)
	default:
		return nil, errors.Errorf("unexpected HIL node type %T", n)
	}
}
