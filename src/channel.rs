use std::cmp;

use tokio::sync::mpsc::{self, Receiver, Sender};
use tokio::sync::{Mutex, Notify};

enum ChannelType {
    Buffered {
        send_port: Sender<isize>,
        recv_port: Mutex<Receiver<isize>>,
    },
    Rendezvous {
        send_port: Mutex<Sender<isize>>,
        recv_port: Mutex<Receiver<isize>>,
        completion_port: Notify,
    },
}

pub struct Channel {
    channel_type: ChannelType,
}

impl Channel {
    pub fn new(capacity: usize) -> Self {
        let (tx, rx) = mpsc::channel(cmp::max(capacity, 1));
        let channel_type = if capacity == 0 {
            ChannelType::Rendezvous {
                send_port: Mutex::new(tx),
                recv_port: Mutex::new(rx),
                completion_port: Notify::new(),
            }
        } else {
            ChannelType::Buffered {
                send_port: tx,
                recv_port: Mutex::new(rx),
            }
        };
        Channel { channel_type }
    }

    pub async fn send(&self, data: isize) {
        match self.channel_type {
            ChannelType::Buffered { ref send_port, .. } => send_port.send(data).await.unwrap(),
            ChannelType::Rendezvous {
                ref send_port,
                ref completion_port,
                ..
            } => {
                let send_port = send_port.lock().await;
                send_port.send(data).await.unwrap();
                completion_port.notified().await
            }
        }
    }

    pub async fn recv(&self) -> Option<isize> {
        match self.channel_type {
            ChannelType::Buffered { ref recv_port, .. } => {
                let mut recv_port = recv_port.lock().await;
                recv_port.recv().await
            }
            ChannelType::Rendezvous {
                ref recv_port,
                ref completion_port,
                ..
            } => {
                let mut recv_port = recv_port.lock().await;
                let data = recv_port.recv().await;
                completion_port.notify_one();
                data
            }
        }
    }
}

#[cfg(test)]
#[cfg(not(miri))]
mod tests {
    use super::*;

    use std::sync::Arc;

    use tokio::runtime::Runtime;

    #[test]
    fn test_buffered_channel_send_recv() {
        let runtime = Runtime::new().unwrap();
        runtime.block_on(async {
            let channel = Channel::new(1);
            channel.send(42).await;
            let data = channel.recv().await;
            assert_eq!(data, Some(42));
        });
    }

    #[test]
    fn test_buffered_channel_order() {
        let runtime = Runtime::new().unwrap();
        runtime.block_on(async {
            let capacity = 10;
            let channel = Channel::new(capacity);
            for i in 0..capacity {
                let data = i as isize;
                channel.send(data).await;
            }
            for i in 0..capacity {
                let data = channel.recv().await;
                assert_eq!(data, Some(i as isize));
            }
        });
    }

    #[test]
    fn test_buffered_channel_primary_send_secondary_recv() {
        let runtime = Runtime::new().unwrap();
        runtime.block_on(async {
            let channel = Arc::new(Channel::new(1));
            let primary = channel.clone();
            let secondary = channel;
            runtime.spawn(async move {
                let data = secondary.recv().await;
                assert_eq!(data, Some(42));
            });
            primary.send(42).await;
        });
    }

    #[test]
    fn test_buffered_channel_primary_recv_secondary_send() {
        let runtime = Runtime::new().unwrap();
        runtime.block_on(async {
            let channel = Arc::new(Channel::new(1));
            let primary = channel.clone();
            let secondary = channel;
            runtime.spawn(async move {
                secondary.send(42).await;
            });
            let data = primary.recv().await;
            assert_eq!(data, Some(42));
        });
    }

    #[test]
    fn test_rendezvous_channel_primary_send_secondary_recv() {
        let runtime = Runtime::new().unwrap();
        runtime.block_on(async {
            let channel = Arc::new(Channel::new(0));
            let primary = channel.clone();
            let secondary = channel;
            runtime.spawn(async move {
                let data = secondary.recv().await;
                assert_eq!(data, Some(42));
            });
            primary.send(42).await;
        });
    }

    #[test]
    fn test_rendezvous_channel_primary_recv_secondary_send() {
        let runtime = Runtime::new().unwrap();
        runtime.block_on(async {
            let channel = Arc::new(Channel::new(0));
            let primary = channel.clone();
            let secondary = channel;
            runtime.spawn(async move {
                secondary.send(42).await;
            });
            let data = primary.recv().await;
            assert_eq!(data, Some(42));
        });
    }
}
