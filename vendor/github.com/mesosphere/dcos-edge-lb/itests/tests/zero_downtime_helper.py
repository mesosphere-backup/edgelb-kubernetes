import json
import queue
import subprocess
import threading

import yaml

import edgelb
import util

MIN_RELOAD_ITERS = 3

logger = util.get_logger(__name__)


class IDApp:
    def __init__(self, base_app):
        self.base_app = base_app
        self.ret_id = util.rand_str(5)
        self.name = base_app + self.ret_id


class StressApp:
    def __init__(self):
        self.name = "stress-app"

    def launch(self):
        with util.ExampleApp("host-mesos-id.json") as c_filename:
            app = None
            with open(c_filename, 'r') as c:
                app = json.load(c)

                app["id"] = self.name
                app["cmd"] = "tail -f /dev/null"
                app["container"]["docker"]["image"] = "nlsun/vegeta"

            with open(c_filename, 'w') as c:
                # Wipe the file
                c.truncate(0)

                s = json.dumps(app)
                logger.info(s)
                c.write(s)
                c.flush()

                util.retry_sh("dcos marathon app add {}".format(c_filename))


class ProxyDef:
    def __init__(self, test_idapp, control_idapp):
        self.testbk = "test-backend"
        self.controlbk = "control-backend"
        self.testport = 81
        self.controlport = 82
        self.test_idapp = test_idapp
        self.control_idapp = control_idapp

        self.config_filename = None

        self.frontends = [
            {
                "bindPort": self.testport,
                "protocol": "HTTP",
                "linkBackend": {"defaultBackend": self.testbk},
            },
            {
                "bindPort": self.controlport,
                "protocol": "HTTP",
                "linkBackend": {"defaultBackend": self.controlbk},
            }
        ]

        self.backends = [
            {
                "name": self.testbk,
                "protocol": "HTTP",
                "servers": [
                    {
                        "framework": {"value": "marathon"},
                        "task": {"value": test_idapp.name},
                        "port": {"name": "id"},
                    }
                ]
            },
            {
                "name": self.controlbk,
                "protocol": "HTTP",
                "servers": [
                    {
                        "framework": {"value": "marathon"},
                        "task": {"value": control_idapp.name},
                        "port": {"name": "id"},
                    }
                ]
            }
        ]


def zero_downtime_reload_tester():
    id0, id1, id2 = mk_idapps()

    proxydef0 = ProxyDef(id0, id1)
    proxydef1 = ProxyDef(id0, id2)

    stress_app = StressApp()
    stress_app.launch()

    poolname = "sample-minimal"
    with util.ExampleConfig("{}.yaml".format(poolname)) as c_filename0, \
            util.ExampleConfig("{}.yaml".format(poolname)) as c_filename1:

        y = None
        with open(c_filename0, 'r') as c:
            y = yaml.load(c)
            y["pools"][0]["haproxy"]["frontends"] = proxydef0.frontends
            y["pools"][0]["haproxy"]["backends"] = proxydef0.backends
        with open(c_filename0, 'w') as c:
            # Wipe the file
            c.truncate(0)

            s = yaml.dump(y)
            logger.info(s)
            c.write(s)
            c.flush()
            proxydef0.config_filename = c_filename0

        with open(c_filename1, 'r') as c:
            y = yaml.load(c)
            y["pools"][0]["haproxy"]["frontends"] = proxydef1.frontends
            y["pools"][0]["haproxy"]["backends"] = proxydef1.backends
        with open(c_filename1, 'w') as c:
            # Wipe the file
            c.truncate(0)

            s = yaml.dump(y)
            logger.info(s)
            c.write(s)
            c.flush()
            proxydef1.config_filename = c_filename1

        run_tester(poolname, proxydef0, proxydef1, stress_app)

    appids = [stress_app.name,
              proxydef0.test_idapp.name,
              proxydef0.control_idapp.name,
              proxydef1.control_idapp.name]
    util.kill_marathon_apps(appids)

    edgelb.delete_pool(poolname)


def run_tester(poolname, proxydef0, proxydef1, stress_app):
    util.retry_sh('dcos edgelb create {}'.format(proxydef0.config_filename))

    lb_host = ("edgelb-pool-0-server.dcos-edgelbpools{}"
               ".autoip.dcos.thisdcos.directory"
               ).format(poolname)

    edgelb.pool_up(poolname, 1)

    tids = edgelb.get_lb_tids(poolname)
    assert len(tids) == 1
    tid = tids[0]

    # Initialize the pool
    load_and_wait(proxydef0, tid, lb_host)

    # The shared state. Thread will append the current number of
    # iterations it's done, then when the stress test is done it will do a
    # read until the queue is empty and take the last value as the final number
    # of iterations.
    counter_q = queue.Queue()

    # Main thread will send a stop signal to this queue
    term_q = queue.Queue()

    # Daemonic thread because it kills suprocesses running within the thread
    # upon crashes.
    reloader = threading.Thread(target=reload_loop,
                                args=(tid, proxydef0, proxydef1, counter_q,
                                      term_q, lb_host),
                                daemon=True)
    reloader.start()

    url = "http://{}:{}/id".format(lb_host, proxydef0.testport)
    stress_output = run_stress(stress_app, url, "90s")

    logger.info("signal term to thread")
    term_q.put(None)
    logger.info("waiting for thread to terminate")
    reloader.join()

    count = None
    while not counter_q.empty():
        count = counter_q.get()
    logger.info("got count {}".format(count))
    assert count >= MIN_RELOAD_ITERS

    logger.info(stress_output)
    assert stress_output["errors"] is None
    for k in stress_output["status_codes"].keys():
        assert k == "200"


def run_stress(stress_app, url, duration):
    cmd = ("""dcos task exec {} """
           """bash -c "echo 'GET {}' | vegeta attack -duration={} | """
           """         vegeta report -reporter json" """
           ).format(stress_app.name, url, duration)
    o = util.retry_sh(cmd, stdout=subprocess.PIPE)
    return json.loads(o.stdout.decode())


def reload_loop(lb_taskid, proxydef0, proxydef1, counter_q, term_q, lb_host):
    logger.info("reload loop")

    count = 0
    # Start with proxydef1 because we loaded it up initially with proxydef0
    proxydefs = [proxydef1, proxydef0]
    while term_q.empty():
        # Update counter first to purposefully underestimate the number of
        # iterations.
        logger.info("update counter {}".format(count))
        counter_q.put(count)

        load_and_wait(proxydefs[0], lb_taskid, lb_host)
        proxydefs.reverse()

        count += 1

    logger.info("reload loop terminated")


def load_and_wait(proxydef, lb_taskid, lb_host):
    util.retry_sh('dcos edgelb update {}'.format(proxydef.config_filename))

    expected = proxydef.control_idapp.ret_id
    got = None
    while True:
        got = query_lb(lb_taskid, lb_host, proxydef.controlport)
        logger.info("got {} expected {}".format(got, expected))
        if got == expected:
            return


def query_lb(lb_taskid, lb_host, port):
    fmts = "dcos task exec {} bash -c 'wget -qO- http://{}:{}/id'"
    o = util.retry_sh(fmts.format(lb_taskid, lb_host, port),
                      stdout=subprocess.PIPE)
    return o.stdout.decode().strip()


def mk_idapps():
    """
    id0: Test app
    id1, id2: Control apps
    """
    idapps = []
    for i in range(3):
        idapp = IDApp("host-mesos-id")
        idapps.append(idapp)
        with util.ExampleApp("{}.json".format(idapp.base_app)) as c_filename:
            app = None
            with open(c_filename, 'r') as c:
                app = json.load(c)

                app["id"] = idapp.name
                app["cmd"] = "{} {}".format(app["cmd"], idapp.ret_id)
                if i == 0:
                    app["instances"] = 2

            with open(c_filename, 'w') as c:
                # Wipe the file
                c.truncate(0)

                s = json.dumps(app)
                logger.info(s)
                c.write(s)
                c.flush()

                util.retry_sh("dcos marathon app add {}".format(c_filename))

    return (idapps[0], idapps[1], idapps[2])
