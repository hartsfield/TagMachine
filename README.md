# TagMachine

NOTE: This program is still in alpha stages and is extremely unstable. Don't 
expect it to be bug free. 

TagMachine is a new type of social media website that aims to accept and 
embrace social media as a modern form of journalism. 

TagMachine requires Redis (6.0.16+) and has only been tested on Linux servers.

To run this program: 

 - clone the repository and `cd` into the project directory
 - Start redis (generally `redis-server &`)
 - run this command with your personal environment variables for the `hmac` sample
secret:

    hmacss=YOUR_SECRET_PHRASE go run .


