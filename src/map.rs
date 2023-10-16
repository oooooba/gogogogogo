use std::collections::HashMap;

use super::type_id::TypeId;

pub(crate) struct Map {
    map: HashMap<usize, usize>,
    _key_type: TypeId,
    _value_type: TypeId,
}

impl Map {
    pub fn new(key_type: TypeId, value_type: TypeId) -> Self {
        Map {
            map: HashMap::new(),
            _key_type: key_type,
            _value_type: value_type,
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
