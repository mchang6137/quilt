const quilt = require('@quilt/quilt');
const infrastructure = require('../../config/infrastructure.js');

const deployment = quilt.createDeployment();
deployment.deploy(infrastructure);

const c = new quilt.Container('iperf', 'networkstatic/iperf3', {
  command: ['-s'],
});

// If we deploy nWorker+1 containers, at least one machine is guaranteed to run
// two containers, and thus be able to test intra-machine bandwidth.
const iperfs = c.replicate(infrastructure.nWorker + 1);
quilt.allow(iperfs, iperfs, 5201);
deployment.deploy(iperfs);
