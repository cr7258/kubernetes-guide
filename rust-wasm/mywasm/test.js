const { echo, new_user } = require('./pkg/mywasm.js');

echo("abc");

const user = new_user(777);
echo(user.get_user_id().toString());