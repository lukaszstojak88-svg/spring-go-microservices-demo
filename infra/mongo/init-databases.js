// Executed by the official mongo image (mongosh) only when the data volume
// is empty, authenticated as the root user.
//
// Creates two logical databases, each with a dedicated user that has
// readWrite access to that database ONLY (decision D9):
//   products_db <- product_user
//   orders_db   <- order_user

function requireEnv(name) {
  const value = process.env[name];
  if (!value) {
    throw new Error(`Missing required environment variable: ${name}`);
  }
  return value;
}

const databases = [
  {
    name: requireEnv("PRODUCT_DB_NAME"),
    user: requireEnv("PRODUCT_DB_USER"),
    password: requireEnv("PRODUCT_DB_PASSWORD"),
  },
  {
    name: requireEnv("ORDER_DB_NAME"),
    user: requireEnv("ORDER_DB_USER"),
    password: requireEnv("ORDER_DB_PASSWORD"),
  },
];

for (const { name, user, password } of databases) {
  const target = db.getSiblingDB(name);

  if (target.getUser(user) === null) {
    target.createUser({
      user,
      pwd: password,
      roles: [{ role: "readWrite", db: name }],
    });
    print(`Created user '${user}' with readWrite on '${name}'`);
  } else {
    print(`User '${user}' already exists on '${name}', skipping`);
  }

  // An empty database does not physically exist in MongoDB; a marker
  // document makes both databases visible right after initialization.
  target.getCollection("_init").updateOne(
    { _id: "init" },
    { $setOnInsert: { createdAt: new Date() } },
    { upsert: true },
  );
}
