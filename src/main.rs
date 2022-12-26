use hcl;
use std::{collections::HashSet, error::Error, fs::File, hash::Hash, str::FromStr};
fn main() -> Result<(), Box<dyn Error>> {
    let f = File::open("./Lakefile")?;
    let val: hcl::Value = hcl::from_reader(f)?;
    println!("{:?}", val);
    println!("{:?}", find_variables(val));
    Ok(())
}

fn find_variables(val: hcl::Value) -> Vec<String> {
    let mut variables = Vec::new();
    match val {
        hcl::Value::Array(vals) => {
            for elem in vals {
                variables.append(&mut find_variables(elem))
            }
        }
        hcl::Value::Object(obj) => {
            for (_, val) in obj.into_iter() {
                variables.append(&mut find_variables(val));
            }
        }
        hcl::Value::String(s) => {
            let tmpl = hcl::Template::from_str(&s).unwrap();
            let elems = tmpl.elements();
            for elem in elems {
                println!("{:?}", elem);
                match elem {
                    hcl::template::Element::Interpolation(int) => {
                        variables.append(&mut find_expression_variables(&int.expr));
                    }
                    hcl::template::Element::Directive(_) => panic!("unimplemented"),
                    _ => {}
                }
            }
        }
        _ => {}
    }
    dedup_vec(variables)
}

fn dedup_vec<T>(v: Vec<T>) -> Vec<T>
where
    T: Eq + Hash,
{
    let mut hash_set = HashSet::new();
    for elem in v {
        hash_set.insert(elem);
    }
    hash_set.into_iter().collect()
}

fn find_expression_variables(expr: &hcl::Expression) -> Vec<String> {
    let mut variables: Vec<String> = Vec::new();
    if let hcl::Expression::Variable(var) = expr {
        variables.push(var.as_str().into());
    }
    let exprs = match expr {
        hcl::Expression::Traversal(t) => vec![&t.expr],
        hcl::Expression::ForExpr(for_exp) => vec![&for_exp.collection_expr],
        hcl::Expression::Conditional(cond_exp) => vec![
            &cond_exp.cond_expr,
            &cond_exp.true_expr,
            &cond_exp.false_expr,
        ],
        _ => vec![],
    };
    for exp in exprs {
        variables.append(&mut find_expression_variables(&exp));
    }

    println!("{:?} -> {:?}", expr, variables);
    variables
}
