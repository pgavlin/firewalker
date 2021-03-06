package il

import (
	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/pkg/util/contract"
)

// A BoundNodeVisitor is a function that visits and optionally replaces a node in a bound property tree.
type BoundNodeVisitor func(n BoundNode) (BoundNode, error)

// IdentityVisitor is a BoundNodeVisitor that returns the input node unchanged.
func IdentityVisitor(n BoundNode) (BoundNode, error) {
	return n, nil
}

func visitBoundArithmetic(n *BoundArithmetic, pre, post BoundNodeVisitor) (BoundNode, error) {
	exprs, err := visitBoundExprs(n.Exprs, pre, post)
	if err != nil {
		return nil, err
	}
	if len(exprs) == 0 {
		return nil, nil
	}
	n.Exprs = exprs
	return post(n)
}

func visitBoundCall(n *BoundCall, pre, post BoundNodeVisitor) (BoundNode, error) {
	exprs, err := visitBoundExprs(n.Args, pre, post)
	if err != nil {
		return nil, err
	}
	n.Args = exprs
	return post(n)
}

func visitBoundConditional(n *BoundConditional, pre, post BoundNodeVisitor) (BoundNode, error) {
	condExpr, err := VisitBoundExpr(n.CondExpr, pre, post)
	if err != nil {
		return nil, err
	}
	trueExpr, err := VisitBoundExpr(n.TrueExpr, pre, post)
	if err != nil {
		return nil, err
	}
	falseExpr, err := VisitBoundExpr(n.FalseExpr, pre, post)
	if err != nil {
		return nil, err
	}
	n.CondExpr, n.TrueExpr, n.FalseExpr = condExpr, trueExpr, falseExpr
	return post(n)
}

func visitBoundIndex(n *BoundIndex, pre, post BoundNodeVisitor) (BoundNode, error) {
	targetExpr, err := VisitBoundExpr(n.TargetExpr, pre, post)
	if err != nil {
		return nil, err
	}
	keyExpr, err := VisitBoundExpr(n.KeyExpr, pre, post)
	if err != nil {
		return nil, err
	}
	n.TargetExpr, n.KeyExpr = targetExpr, keyExpr
	return post(n)
}

func visitBoundListProperty(n *BoundListProperty, pre, post BoundNodeVisitor) (BoundNode, error) {
	exprs, err := visitBoundNodes(n.Elements, pre, post)
	if err != nil {
		return nil, err
	}
	if len(exprs) == 0 {
		return nil, nil
	}
	n.Elements = exprs
	return post(n)
}

func visitBoundMapProperty(n *BoundMapProperty, pre, post BoundNodeVisitor) (BoundNode, error) {
	for k, e := range n.Elements {
		ee, err := VisitBoundNode(e, pre, post)
		if err != nil {
			return nil, err
		}
		if ee == nil {
			delete(n.Elements, k)
		} else {
			n.Elements[k] = ee
		}
	}
	return post(n)
}

func visitBoundOutput(n *BoundOutput, pre, post BoundNodeVisitor) (BoundNode, error) {
	exprs, err := visitBoundExprs(n.Exprs, pre, post)
	if err != nil {
		return nil, err
	}
	if len(exprs) == 0 {
		return nil, nil
	}
	n.Exprs = exprs
	return post(n)
}

func visitBoundExprs(ns []BoundExpr, pre, post BoundNodeVisitor) ([]BoundExpr, error) {
	nils := 0
	for i, e := range ns {
		ee, err := VisitBoundExpr(e, pre, post)
		if err != nil {
			return nil, err
		}
		if ee == nil {
			nils++
		}
		ns[i] = ee
	}
	if nils == 0 {
		return ns, nil
	} else if nils == len(ns) {
		return []BoundExpr{}, nil
	}

	nns := make([]BoundExpr, 0, len(ns)-nils)
	for _, e := range ns {
		if e != nil {
			nns = append(nns, e)
		}
	}
	return nns, nil
}

func visitBoundNodes(ns []BoundNode, pre, post BoundNodeVisitor) ([]BoundNode, error) {
	nils := 0
	for i, e := range ns {
		ee, err := VisitBoundNode(e, pre, post)
		if err != nil {
			return nil, err
		}
		if ee == nil {
			nils++
		}
		ns[i] = ee
	}
	if nils == 0 {
		return ns, nil
	} else if nils == len(ns) {
		return []BoundNode{}, nil
	}

	nns := make([]BoundNode, 0, len(ns)-nils)
	for _, e := range ns {
		if e != nil {
			nns = append(nns, e)
		}
	}
	return nns, nil
}

// VisitBoundNode visits each node in a property tree using the given pre- and post-order visitors. If the preorder
// visitor returns a new node, that node's descendents will be visited. This function returns the result of the
// post-order visitor. If any visitor returns an error, the walk halts and that error is returned.
func VisitBoundNode(n BoundNode, pre, post BoundNodeVisitor) (BoundNode, error) {
	nn, err := pre(n)
	if err != nil {
		return nil, err
	}
	n = nn

	switch n := n.(type) {
	case *BoundArithmetic:
		return visitBoundArithmetic(n, pre, post)
	case *BoundCall:
		return visitBoundCall(n, pre, post)
	case *BoundConditional:
		return visitBoundConditional(n, pre, post)
	case *BoundIndex:
		return visitBoundIndex(n, pre, post)
	case *BoundListProperty:
		return visitBoundListProperty(n, pre, post)
	case *BoundLiteral:
		return post(n)
	case *BoundMapProperty:
		return visitBoundMapProperty(n, pre, post)
	case *BoundOutput:
		return visitBoundOutput(n, pre, post)
	case *BoundVariableAccess:
		return post(n)
	default:
		contract.Failf("unexpected node type in visitBoundExpr: %T", n)
		return nil, errors.Errorf("unexpected node type in visitBoundExpr: %T", n)
	}
}

// VisitBoundExpr visits each node in an expression tree using the given pre- and post-order visitors. Its behavior is
// identical to that of VisitBoundNode, but it requires that the given visitors return BoundExpr values.
func VisitBoundExpr(n BoundExpr, pre, post BoundNodeVisitor) (BoundExpr, error) {
	nn, err := VisitBoundNode(n, pre, post)
	if err != nil || nn == nil {
		return nil, err
	}
	return nn.(BoundExpr), nil
}
