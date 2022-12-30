use daggy;
use daggy::{
    petgraph,
    petgraph::dot::{Config, Dot},
    petgraph::visit::IntoNeighborsDirected,
    Dag, NodeIndex,
};
use hcl;
use std::{
    collections::{HashMap, HashSet, VecDeque},
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
    // let obj = val.as_object().unwrap();
    // for (k, v) in obj {
    //     println!("{:?} => {:?}", k, v);
    // }
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
    fn parents(&mut self, next: NodeIndex) -> petgraph::graph::Neighbors<()> {
        self.dag
            .neighbors_directed(next, petgraph::Direction::Incoming)
    }
    fn children(&mut self, next: NodeIndex) -> petgraph::graph::Neighbors<()> {
        self.dag
            .neighbors_directed(next, petgraph::Direction::Outgoing)
    }

    fn walk<F>(&mut self, callback: F)
    where
        F: Fn(&String),
    {
        // let sorted = petgraph::algo::toposort(&self.dag, None).unwrap();
        // println!("{:?}", sorted);
        let node_count = self.dag.node_count();

        // Vec with parents of each node. We also use this to track if we've
        // processed a node. We delete the parents once we've visited the
        // corresponding node.
        let mut parents: Vec<Option<Vec<NodeIndex>>> = Vec::with_capacity(node_count);

        // Queue for nodes to process
        let mut queue: VecDeque<NodeIndex> = VecDeque::with_capacity(node_count);

        for i in 0..node_count {
            let idx = NodeIndex::new(i);
            let node_parents: Vec<NodeIndex> = self.parents(idx).collect();
            parents.push(Some(node_parents));
            queue.push_back(idx);
        }
        queue = {
            // Reverse queue
            let mut v: Vec<NodeIndex> = queue.into();
            v.reverse();
            v.into()
        };

        while !queue.is_empty() {
            // Get next node
            let next = queue.pop_front().unwrap();

            // Skip if it's complete
            if parents[next.index()].is_none() {
                continue;
            }

            // Check if each parent is complete, if so we can proceed
            let all_parents_are_complete = parents[next.index()]
                .as_ref()
                .unwrap()
                .into_iter()
                .map(|parent| -> bool { parents[parent.index()].is_none() })
                .all(|b| b);

            if !all_parents_are_complete {
                println!("Skipping {:?}", next);
                continue;
            }

            // If all parents are complete, run the callback with the node value.
            let node_value = self.dag.node_weight(next).unwrap();
            callback(&node_value);

            // Mark node as complete.
            parents[next.index()] = None;

            // Stick any not completed children we might have skipped back on
            // the queue. Maybe they can be processed now.
            for child in self.children(next) {
                queue.push_back(child);
            }
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
                println!("{:?}", elem);
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