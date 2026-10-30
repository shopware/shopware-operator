## Faulty messages

Currently 1 Customer sends 1 faulty message into the queue and this leads to endless loop of consuming this message

### The Problem

This is not easy solvable without touching php code.
Technicaly speaking this should be solved by shopware if they can.
If a message has errors normaly it's already moved to a different queue, so this is then not a problem.
But memory issues are a problem in php for the consumers.

### What is not Not working

- Fatal errors (like the PHP memory limit) bypass Messenger's exception handling

## Queue FIFO

The queue will pile up because of FIFO

## Worker ooms, data loss

When the worker has to much messages, the worker can't even get messages and also new messages can't be published

# Solutions

## Worker ooms, data loss / FIFO queue -> Use SQS/Rabbitmq

Dead letter queue support, if a message gets delivered to often we can move them into a different queue.
Then we have a deployment which consumes these messages and has higher memory limits, if they then also
fail, we stop completly processing.

SQS -> Easy, but no access for customers
Nats -> Not supported by symfony nativly, but there is a external one, maybe poc
Rabbitmq -> Not easy, but access for messages for customers, shopware standard
Kafka -> Yeah, no

### Problems

- No access to messages, only through shopware. But with froshtools it's a bit extendet and you
have more options. But need to align
- Need to check if we can delete messages, but I think we can't

## Alerting for the customer

We need to die somewhere:

1. Queue doesn't except messages -> Data loss (Not working)
2. Queue is filled with a million messages -> Data is unsync when we have less consumers
3. High consumer scaling -> Database can die

#### Active alerting

So we should at least have somewhere a notification system, so we do alerting for customers.
Could be to have a slack bot connected with there grafana so we post messages in there channel in the end.

PaaS system -> Use a notification table to highlight issues for each command with the application.

```
❯ sw-paas app update
Notification: Your system has currently 17.234 Messages in async queue. This doesn't look normal.

Selected: derks (b8f5149a-5c61-4c2b-8168-1a05068ae7bf)
Found one project: customer (936d2f31-324f-43a9-9fdf-aab295c2a026)
Found one application: amazing-hs (42ef1ef6-9dce-4974-8c5c-7a075e3fb734)
Enter the commit-sha

> 0edffd8e6b17caf8db2b3dcf02b452f0b016ea98

(esc to quit)
Error: failed to get commit-sha: aborted by User
```

# TODO

## Queue

- Decide between SQS and RabbitMQ as message transport
  - Ask 8mylez why they want RabbitMQ instead of SQS. Which features do they need?
  - Decision depends on the answer: SQS (easy, no customer access) or RabbitMQ (harder, customer access to messages)
- Migrate the shopware-operator to the new transport
  - Problem: a message handler can put a new message into the queue. Check that no messages get lost during migration
- Enable cleanup of orphaned files in S3
- Add alerting when too many messages are in the queue
- Update docs: how to define new queues

## PaaS backend: enforce recipes

- Make recipes enforceable
  - Use shopware-cli for this
  - Use the recipes update command
  - Provide the supported recipe version as an environment variable
- POC: Symfony bundle instead of recipes
  - Config lives in PHP code, so we can handle different Shopware versions with simple if/else
  - Soner had the same issues with recipes, so we need this POC

## Grafana

- Enable re-alerting (repeat notifications while an alert is still firing)
- Add custom monitors, e.g. for queue size
- Send alerts to Slack by default when a Slack secret is provided in Vault
