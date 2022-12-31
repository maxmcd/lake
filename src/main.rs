use daggy;
use daggy::{
    petgraph,
    petgraph::dot::{Config, Dot},
    Dag, NodeIndex,
};
use hcl;
use std::{
    collections::{HashMap, HashSet},
    error::Error,
    fmt,
    fs::File,
    hash::Hash,
    str::FromStr,
};

type Result<T> = std::result::Result<T, Box<dyn Error>>;

fn main() -> Result<()> {
    let f = File::open("./Lakefile")?;
    let body: hcl::Body = hcl::from_reader(f)?;
    println!("{:?}", body);
    parse_body(body)?;
    Ok(())
}

fn parse_body(body: hcl::Body) -> Result<()> {
    let mut dag = UniqueDag::new();
    let mut name_set: HashSet<String> = HashSet::new();

    for thing in body.into_inner() {
        let (name, variables) = match thing {
            hcl::Structure::Block(block) => {
                if block.identifier.as_str() == "defaults" {
                    continue;
                }
                let name = validate_block_type(&block)?;

                let value: hcl::Value = block.into();
                (name, find_variables(&value))
            }
            hcl::Structure::Attribute(attr) => {
                let name: String = attr.key.as_str().into();
                let value: hcl::Value = attr.into();
                (name, find_variables(&value))
            }
        };
        if name_set.contains(&name) {
            return Err(validation_error(format!("Found duplicate name {name}")));
        }
        name_set.insert(name.clone());
        dag.add(name, variables)?;
    }
    println!("{:?}", Dot::with_config(&dag.dag, &[Config::EdgeNoLabel]));

    dag.walk(|thing| println!("{thing}"));

    Ok(())
}

struct UniqueDag {
    dag: Dag<String, ()>,
    node_indexes: HashMap<String, daggy::NodeIndex>,
}

impl UniqueDag {
    fn new() -> Self {
        Self {
            dag: Dag::new(),
            node_indexes: HashMap::new(),
        }
    }
    fn add(&mut self, name: String, variables: Vec<String>) -> Result<()> {
        let name_idx = self.add_node(name);
        for var in variables {
            let var_idx = self.add_node(var);
            // TODO: better error?
            self.dag.add_edge(var_idx, name_idx, ())?;
        }
        Ok(())
    }

    fn add_node(&mut self, val: String) -> NodeIndex {
        if let Some(idx) = self.node_indexes.get(&val) {
            *idx
        } else {
            let idx = self.dag.add_node(val.clone());
            self.node_indexes.insert(val, idx);
            idx
        }
    }

    fn walk<F>(&mut self, callback: F)
    where
        F: Fn(&String),
    {
        for idx in petgraph::algo::toposort(&self.dag, None).unwrap() {
            let node_value = self.dag.node_weight(idx).unwrap();
            callback(&node_value);
        }
    }
}

fn validate_block_type(block: &hcl::Block) -> Result<String> {
    let identifier_str = block.identifier.as_str();
    match identifier_str {
        "command" | "file" | "store" => {}
        _ => {
            return Err(Box::new(InvalidBlockError {
                name: block.identifier.to_owned(),
            }))
        }
    };
    if block.labels().len() != 1 {
        return Err(validation_error(format!(
            "block '{identifier_str}' has an incorrect number of labels"
        )));
    }
    Ok(block.labels.first().unwrap().to_owned().into_inner())
}

fn block_to_recipe(block: &hcl::Block) -> Result<Recipe> {
    let name = validate_block_type(block)?;

    let mut recipe = Recipe {
        env: HashMap::new(),
        inputs: Vec::new(),
        is_store: block.identifier.as_str() == "store",
        name: name,
        // Parse
        network: false,
        script: String::new(),
        shell: Vec::new(),
    };

    // for attr in block.body.attributes() {
    //     let attr_key_str = attr.key.as_str();
    //     match attr_key_str {
    //         "name" => {
    //             recipe.name = ;
    //         }
    //         "i"
    //         _ => {}
    //     }
    // }

    Ok(recipe)
}

#[derive(Debug, Clone)]
struct InvalidBlockError {
    name: hcl::Identifier,
}

impl Error for InvalidBlockError {}

impl fmt::Display for InvalidBlockError {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        write!(f, "unexpected block '{}'", self.name)
    }
}

#[derive(Debug, Clone)]
struct ValidationError {
    message: String,
}

impl Error for ValidationError {}

impl fmt::Display for ValidationError {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        write!(f, "{}", self.message)
    }
}

fn validation_error(msg: String) -> Box<dyn Error> {
    Box::new(ValidationError { message: msg })
}

struct Recipe {
    env: HashMap<String, String>,
    inputs: Vec<String>,
    is_store: bool,
    name: String,
    network: bool,
    script: String,
    shell: Vec<String>,
}

fn find_variables(val: &hcl::Value) -> Vec<String> {
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
                // println!("{:?}", elem);
                match elem {
                    hcl::template::Element::Interpolation(int) => {
                        find_expression_variables(&mut variables, &int.expr);
                    }
                    hcl::template::Element::Directive(_) => panic!("unimplemented"),
                    hcl::template::Element::Literal(_) => {}
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
    // TODO: better?
    let mut hash_set = HashSet::new();
    for elem in v {
        hash_set.insert(elem);
    }
    hash_set.into_iter().collect()
}

fn find_expression_variables(variables: &mut Vec<String>, expr: &hcl::Expression) {
    if let hcl::Expression::Variable(var) = expr {
        variables.push(var.as_str().into());
        return;
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
        find_expression_variables(variables, &exp);
    }
}

#[cfg(test)]
mod tests {
    use crate::{parse_body, Result};
    use hcl;
    use std::fs::File;

    fn get_err_contains(block: &hcl::Body) -> String {
        for attr in block.attributes() {
            if attr.key.clone().into_inner() == "err_contains" {
                if let hcl::Expression::String(str) = attr.expr.clone() {
                    return str;
                } else {
                    panic!(
                        "err_contains expression is not the correct type: {:?}",
                        attr.expr
                    )
                }
            } else {
                panic!("Unexpected attribute found: {:#?}", attr);
            }
        }
        String::new()
    }
    fn get_lakefile(block: &hcl::Body) -> Option<hcl::Body> {
        for block in block.blocks() {
            if block.identifier.clone().into_inner() == "file" {
                return Some(block.body.clone());
            }
        }
        None
    }

    #[test]
    fn test_hcl() -> Result<()> {
        // Tests run from proj root
        let f = File::open("./src/test.hcl")?;
        let body: hcl::Body = hcl::from_reader(f)?;
        for entity in body.into_inner() {
            let (name, err_contains, lakefile) = match entity {
                hcl::Structure::Attribute(attr) => {
                    panic!("Attributes are not allowed in test.hcl: {:?}", attr)
                }
                hcl::Structure::Block(block) => (
                    block.labels.first().unwrap().clone().into_inner(),
                    get_err_contains(&block.body),
                    get_lakefile(&block.body).expect("lakefile should exist within test block"),
                ),
            };

            println!("\nRunning test: {:#?}", name);
            if err_contains != "" {
                println!("\tConfirming that err contains: {:#?}", err_contains);
            }

            let result = parse_body(lakefile.into());
            if err_contains != "" && result.is_ok() {
                panic!(
                    "Test {name} was expected to return an error containing {:?}, but no error was found",
                    err_contains
                )
            } else if result.is_err() {
                let err_msg = format!("{:?}", result.err().unwrap());
                if !err_msg.contains(&err_contains) {
                    panic!(
                        "\n\nTest {name} was expected to return an error containing:\n\t{:?}\nit instead returned the error:\n\t{:?}\n",
                        err_contains,
                        err_msg,
                    )
                }
            }
        }
        Ok(())
    }
}
