use std::collections::VecDeque;

enum ChannelType {
    Buffered {
        buffer: VecDeque<isize>,
        capacity: usize,
    },
    Rendezvous {
        buffer: Option<isize>,
        has_received: bool,
    },
}

pub struct Channel {
    channel_type: ChannelType,
}

impl Channel {
    pub fn new(capacity: usize) -> Self {
        let channel_type = if capacity == 0 {
            ChannelType::Rendezvous {
                buffer: None,
                has_received: false,
            }
        } else {
            ChannelType::Buffered {
                buffer: VecDeque::with_capacity(capacity),
                capacity,
            }
        };
        Channel { channel_type }
    }

    pub fn send(&mut self, data: &isize) -> Option<()> {
        match self.channel_type {
            ChannelType::Buffered {
                ref mut buffer,
                capacity,
            } => {
                let len = buffer.len();
                assert!(len <= capacity);
                if len < capacity {
                    buffer.push_back(*data);
                    Some(())
                } else {
                    None
                }
            }
            ChannelType::Rendezvous {
                ref mut buffer,
                ref mut has_received,
            } => {
                match (buffer.is_some(), *has_received) {
                    (false, false) => {
                        *buffer = Some(*data);
                        None
                    }
                    (false, true) => {
                        // ToDo: The following sequence causes inconsistencies.
                        // sender0 : send data X
                        // receiver : receive data X
                        // sender1 : send data Y (reaches here. sender1 does not send data Y. sender1 must be blocked until send0 reaches here)
                        *has_received = false;
                        Some(())
                    }
                    (true, false) => None,
                    (true, true) => unreachable!(),
                }
            }
        }
    }

    pub fn recv(&mut self) -> Option<isize> {
        match self.channel_type {
            ChannelType::Buffered { ref mut buffer, .. } => buffer.pop_front(),
            ChannelType::Rendezvous {
                ref mut buffer,
                ref mut has_received,
            } => {
                let data = buffer.take()?;
                assert!(!*has_received);
                *has_received = true;
                Some(data)
            }
        }
    }
}

#[cfg(test)]
#[cfg(not(miri))]
mod tests {
    use super::*;

    use std::cell::RefCell;
    use std::rc::Rc;

    #[test]
    fn test_buffered_channel_send_recv() {
        let mut channel = Channel::new(1);
        let success = channel.send(&42);
        assert_eq!(success, Some(()));
        let data = channel.recv();
        assert_eq!(data, Some(42));
    }

    #[test]
    fn test_buffered_channel_order() {
        let capacity = 10;
        let mut channel = Channel::new(capacity);
        for i in 0..capacity {
            let data = i as isize;
            channel.send(&data).unwrap();
        }
        for i in 0..capacity {
            let data = channel.recv();
            assert_eq!(data, Some(i as isize));
        }
    }

    #[test]
    fn test_buffered_channel_primary_send_secondary_recv() {
        let channel = Rc::new(RefCell::new(Channel::new(1)));
        let primary = channel.clone();
        let secondary = channel;
        {
            let success = primary.borrow_mut().send(&42);
            assert_eq!(success, Some(()));
        }
        {
            let data = secondary.borrow_mut().recv();
            assert_eq!(data, Some(42));
        }
    }

    #[test]
    fn test_buffered_channel_primary_recv_secondary_send() {
        let channel = Rc::new(RefCell::new(Channel::new(1)));
        let primary = channel.clone();
        let secondary = channel;
        {
            let data = primary.borrow_mut().recv();
            assert_eq!(data, None);
        }
        {
            let success = secondary.borrow_mut().send(&42);
            assert_eq!(success, Some(()));
        }
    }

    #[test]
    fn test_rendezvous_channel_primary_send_begin_secondary_recv_primary_send_end() {
        let channel = Rc::new(RefCell::new(Channel::new(0)));
        let primary = channel.clone();
        let secondary = channel;
        let data = 42;
        {
            let success = primary.borrow_mut().send(&data);
            assert_eq!(success, None);
        }
        {
            let data = secondary.borrow_mut().recv();
            assert_eq!(data, Some(42));
        }
        {
            let success = primary.borrow_mut().send(&data);
            assert_eq!(success, Some(()));
        }
    }

    #[test]
    fn test_rendezvous_channel_primary_recv_secondary_send() {
        let channel = Rc::new(RefCell::new(Channel::new(0)));
        let primary = channel.clone();
        let secondary = channel;
        {
            let data = primary.borrow_mut().recv();
            assert_eq!(data, None);
        }
        {
            let success = secondary.borrow_mut().send(&42);
            assert_eq!(success, None);
        }
    }
}
