use std::collections::HashMap;

pub struct Map {
    map: HashMap<usize, usize>,
}

impl Map {
    pub fn new(_key_type_size: usize, _value_type_size: usize) -> Self {
        Map {
            map: HashMap::new(),
        }
    }

    pub fn len(&self) -> usize {
        self.map.len()
    }
}
