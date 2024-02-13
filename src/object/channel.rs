use std::mem;
use std::slice;

use crate::ObjectAllocator;
use crate::ObjectPtr;

#[derive(Debug)]
pub(crate) enum ReceiveStatus<T> {
    Value(T),
    Blocked,
    Closed,
}

struct BufferedChannel {
    buffer: *mut ObjectPtr,
    capacity: usize,
    head: usize,
    size: usize,
}

impl BufferedChannel {
    pub fn new(capacity: usize, allocator: &mut dyn ObjectAllocator) -> Self {
        assert!(capacity > 0);
        let size = mem::size_of::<ObjectPtr>() * capacity;
        let buffer = allocator.allocate(size, |_| {}) as *mut ObjectPtr;
        Self {
            buffer,
            capacity,
            head: 0,
            size: 0,
        }
    }

    fn at(&self, index: usize) -> &ObjectPtr {
        let buffer = unsafe { slice::from_raw_parts(self.buffer, self.capacity) };
        &buffer[index]
    }

    fn at_mut(&mut self, index: usize) -> &mut ObjectPtr {
        let buffer = unsafe { slice::from_raw_parts_mut(self.buffer, self.capacity) };
        &mut buffer[index]
    }

    pub fn send(&mut self, data: ObjectPtr) -> Option<()> {
        assert!(self.size <= self.capacity);
        if self.size >= self.capacity {
            return None;
        }

        let tail = (self.head + self.size) % self.capacity;
        *self.at_mut(tail) = data;
        self.size += 1;

        Some(())
    }

    pub fn receive(&mut self) -> Option<ObjectPtr> {
        if self.size == 0 {
            return None;
        }

        let data = self.at(self.head).clone();
        self.head = (self.head + 1) % self.capacity;
        self.size -= 1;

        Some(data)
    }
}

struct RendezvousChannel {
    sender_id: Option<usize>,
    data: Option<ObjectPtr>,
}

impl RendezvousChannel {
    pub fn new() -> Self {
        Self {
            sender_id: None,
            data: None,
        }
    }

    pub fn send(&mut self, id: usize, data: ObjectPtr) -> Option<()> {
        if self.data.is_some() {
            return None;
        }

        match self.sender_id {
            Some(sender_id) if sender_id == id => {
                self.sender_id = None;
                Some(())
            }
            Some(_) => None,
            None => {
                self.data = Some(data);
                self.sender_id = Some(id);
                None
            }
        }
    }

    pub fn receive(&mut self) -> Option<ObjectPtr> {
        self.data.take()
    }
}

enum ChannelType {
    Buffered(BufferedChannel),
    Rendezvous(RendezvousChannel),
}

pub(crate) struct ChannelObject {
    channel_type: ChannelType,
    is_closed: bool,
}

impl ChannelObject {
    pub fn new(capacity: usize, allocator: &mut dyn ObjectAllocator) -> Self {
        let channel_type = if capacity > 0 {
            ChannelType::Buffered(BufferedChannel::new(capacity, allocator))
        } else {
            ChannelType::Rendezvous(RendezvousChannel::new())
        };
        Self {
            channel_type,
            is_closed: false,
        }
    }

    pub fn close(&mut self, _id: usize) {
        assert!(!self.is_closed);
        self.is_closed = true;
    }

    pub fn send(&mut self, id: usize, data: ObjectPtr) -> Option<()> {
        match self.channel_type {
            ChannelType::Buffered(ref mut channel) => channel.send(data),
            ChannelType::Rendezvous(ref mut channel) => channel.send(id, data),
        }
    }

    pub fn receive(&mut self, _id: usize) -> ReceiveStatus<ObjectPtr> {
        let result = match self.channel_type {
            ChannelType::Buffered(ref mut channel) => channel.receive(),
            ChannelType::Rendezvous(ref mut channel) => channel.receive(),
        };
        if let Some(data) = result {
            ReceiveStatus::Value(data)
        } else if self.is_closed {
            ReceiveStatus::Closed
        } else {
            ReceiveStatus::Blocked
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    use std::cell::RefCell;
    use std::ptr;
    use std::rc::Rc;

    impl<T> PartialEq for ReceiveStatus<T> {
        fn eq(&self, other: &Self) -> bool {
            use ReceiveStatus::*;
            match (self, other) {
                (Blocked, Blocked) => true,
                (Closed, Closed) => true,
                _ => false,
            }
        }
    }

    impl<T> Eq for ReceiveStatus<T> {}

    struct AllocatedObject {
        ptr: *mut (),
        size: usize,
        destructor: fn(*mut ()),
    }

    struct MockObjectAllocator {
        allocated_objects: Vec<AllocatedObject>,
    }

    impl MockObjectAllocator {
        fn new() -> Self {
            MockObjectAllocator {
                allocated_objects: Vec::new(),
            }
        }
    }

    impl ObjectAllocator for MockObjectAllocator {
        fn allocate(&mut self, size: usize, destructor: fn(*mut ())) -> *mut () {
            let alignment = mem::align_of::<isize>();
            let size = (size + alignment - 1) / alignment * alignment;
            let buf: Vec<isize> = vec![0; size];
            let ptr = buf.leak().as_mut_ptr() as *mut ();
            self.allocated_objects.push(AllocatedObject {
                ptr,
                size,
                destructor,
            });
            ptr
        }

        fn allocate_guarded_pages(&mut self, _num_pages: usize) -> *mut () {
            unimplemented!()
        }
    }

    impl Drop for MockObjectAllocator {
        fn drop(&mut self) {
            for allocated_object in &self.allocated_objects {
                (allocated_object.destructor)(allocated_object.ptr);
                unsafe {
                    Vec::from_raw_parts(
                        allocated_object.ptr as *mut isize,
                        0,
                        allocated_object.size,
                    );
                }
            }
        }
    }

    #[test]
    fn test_buffered_channel_send_receive() {
        let mut allocator = MockObjectAllocator::new();
        let channel = Rc::new(RefCell::new(ChannelObject::new(1, &mut allocator)));
        {
            let data = allocator.allocate(mem::size_of::<isize>(), |_| {}) as *mut isize;
            unsafe { *data = 42 };
            let data = ObjectPtr(data as *mut ());
            let result = channel.borrow_mut().send(1, data);
            assert_eq!(result, Some(()));
        }
        {
            let result = channel.borrow_mut().receive(1);
            let ReceiveStatus::Value(data) = result else {
                panic!()
            };
            assert_eq!(*data.as_ref::<isize>(), 42);
        }
    }

    #[test]
    fn test_buffered_channel_order() {
        let capacity = 10;
        let mut allocator = MockObjectAllocator::new();
        let channel = Rc::new(RefCell::new(ChannelObject::new(capacity, &mut allocator)));
        for i in 0..capacity {
            let data = allocator.allocate(mem::size_of::<isize>(), |_| {}) as *mut isize;
            unsafe { *data = i as isize };
            let data = ObjectPtr(data as *mut ());
            let result = channel.borrow_mut().send(1, data);
            assert_eq!(result, Some(()));
        }
        for i in 0..capacity {
            let result = channel.borrow_mut().receive(1);
            let ReceiveStatus::Value(data) = result else {
                panic!()
            };
            assert_eq!(*data.as_ref::<usize>(), i);
        }
    }

    #[test]
    fn test_buffered_channel_first_send_second_receive() {
        let mut allocator = MockObjectAllocator::new();
        let channel = Rc::new(RefCell::new(ChannelObject::new(1, &mut allocator)));
        let first = channel.clone();
        let second = channel;
        {
            let data = allocator.allocate(mem::size_of::<isize>(), |_| {}) as *mut isize;
            unsafe { *data = 42 };
            let data = ObjectPtr(data as *mut ());
            let result = first.borrow_mut().send(1, data);
            assert_eq!(result, Some(()));
        }
        {
            let result = second.borrow_mut().receive(2);
            let ReceiveStatus::Value(data) = result else {
                panic!()
            };
            assert_eq!(*data.as_ref::<isize>(), 42);
        }
    }

    #[test]
    fn test_buffered_channel_first_receive_second_send() {
        let mut allocator = MockObjectAllocator::new();
        let channel = Rc::new(RefCell::new(ChannelObject::new(1, &mut allocator)));
        let first = channel.clone();
        let second = channel;
        {
            let result = first.borrow_mut().receive(1);
            assert_eq!(result, ReceiveStatus::Blocked);
        }
        {
            let data = allocator.allocate(mem::size_of::<isize>(), |_| {}) as *mut isize;
            unsafe { *data = 42 };
            let data = ObjectPtr(data as *mut ());
            let result = second.borrow_mut().send(2, data);
            assert_eq!(result, Some(()));
        }
    }

    #[test]
    fn test_buffered_channel_send_close_receive() {
        let mut allocator = MockObjectAllocator::new();
        let channel = Rc::new(RefCell::new(ChannelObject::new(1, &mut allocator)));
        {
            let data = allocator.allocate(mem::size_of::<isize>(), |_| {}) as *mut isize;
            unsafe { *data = 42 };
            let data = ObjectPtr(data as *mut ());
            let result = channel.borrow_mut().send(1, data);
            assert_eq!(result, Some(()));
        }
        {
            channel.borrow_mut().close(1);
        }
        {
            let result = channel.borrow_mut().receive(1);
            let ReceiveStatus::Value(data) = result else {
                panic!()
            };
            assert_eq!(*data.as_ref::<isize>(), 42);
        }
        {
            let result = channel.borrow_mut().receive(1);
            assert_eq!(result, ReceiveStatus::Closed);
        }
    }

    #[test]
    fn test_rendezvous_channel_first_send_second_receive() {
        let mut allocator = MockObjectAllocator::new();
        let channel = Rc::new(RefCell::new(ChannelObject::new(0, &mut allocator)));
        let first = channel.clone();
        let second = channel;
        {
            let data = allocator.allocate(mem::size_of::<isize>(), |_| {}) as *mut isize;
            unsafe { *data = 42 };
            let data = ObjectPtr(data as *mut ());
            let result = first.borrow_mut().send(1, data);
            assert_eq!(result, None);
        }
        {
            let result = second.borrow_mut().receive(2);
            let ReceiveStatus::Value(data) = result else {
                panic!()
            };
            assert_eq!(*data.as_ref::<isize>(), 42);
        }
        {
            let data = ObjectPtr(ptr::null_mut() as *mut ());
            let result = first.borrow_mut().send(1, data);
            assert_eq!(result, Some(()));
        }
    }

    #[test]
    fn test_rendezvous_channel_first_receive_second_send() {
        let mut allocator = MockObjectAllocator::new();
        let channel = Rc::new(RefCell::new(ChannelObject::new(0, &mut allocator)));
        let first = channel.clone();
        let second = channel;
        {
            let result = first.borrow_mut().receive(1);
            assert_eq!(result, ReceiveStatus::Blocked);
        }
        {
            let data = allocator.allocate(mem::size_of::<isize>(), |_| {}) as *mut isize;
            unsafe { *data = 42 };
            let data = ObjectPtr(data as *mut ());
            let result = second.borrow_mut().send(2, data);
            assert_eq!(result, None);
        }
        {
            let result = first.borrow_mut().receive(1);
            let ReceiveStatus::Value(data) = result else {
                panic!()
            };
            assert_eq!(*data.as_ref::<isize>(), 42);
        }
        {
            let data = ObjectPtr(ptr::null_mut() as *mut ());
            let result = second.borrow_mut().send(2, data);
            assert_eq!(result, Some(()));
        }
    }

    #[test]
    fn test_rendezvous_channel_first_send_and_close_second_receive() {
        let mut allocator = MockObjectAllocator::new();
        let channel = Rc::new(RefCell::new(ChannelObject::new(0, &mut allocator)));
        let first = channel.clone();
        let second = channel;
        {
            let data = allocator.allocate(mem::size_of::<isize>(), |_| {}) as *mut isize;
            unsafe { *data = 42 };
            let data = ObjectPtr(data as *mut ());
            let result = first.borrow_mut().send(1, data);
            assert_eq!(result, None);
        }
        {
            first.borrow_mut().close(1);
        }
        {
            let result = second.borrow_mut().receive(2);
            let ReceiveStatus::Value(data) = result else {
                panic!()
            };
            assert_eq!(*data.as_ref::<isize>(), 42);
        }
        {
            let result = second.borrow_mut().receive(2);
            assert_eq!(result, ReceiveStatus::Closed);
        }
    }
}
