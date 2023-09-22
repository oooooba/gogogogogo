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

    pub fn get(&self, key: &usize) -> Option<usize> {
        self.map.get(key).copied()
    }

    pub fn set(&mut self, key: &usize, value: usize) {
        self.map.insert(*key, value);
    }
}
