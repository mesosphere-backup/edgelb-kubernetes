import json
import subprocess

import retrying
import yaml

import edgelb
import util

logger = util.get_logger(__name__)


class Counter:
    def __init__(self):
        self.n = 0

    def count(self):
        tmp = self.n
        self.n += 1
        return tmp


class AppObj:
    ST_AUTO = 1
    ST_AGENT = 2
    ST_CONTAINER = 3
    ST_STATIC_VIP = 4
    ST_NAMED_VIP = 5

    def __init__(self, n, name):
        self.name = name
        self.ret_id = util.rand_str(5)
        self.named_vip = "/{}:80".format(self.name)
        self.static_vip = "1.1.1.{}:81".format(n)
        self.portlabels = {
            "VIP_0": self.named_vip,
            "VIP_1": self.static_vip,
        }

    def backend_server(self, server_type):
        sv = {
            "framework": {"value": "marathon"},
            "task": {"value": self.name},
            "port": {"name": "id"},
        }
        if server_type == self.ST_AUTO:
            sv["type"] = "AUTO_IP"
            return sv
        if server_type == self.ST_AGENT:
            sv["type"] = "AGENT_IP"
            return sv
        if server_type == self.ST_CONTAINER:
            sv["type"] = "CONTAINER_IP"
            return sv
        if server_type == self.ST_STATIC_VIP:
            del sv["task"]
            sv["type"] = "VIP"
            sv["port"] = {"vip": self.static_vip}
            return sv
        if server_type == self.ST_NAMED_VIP:
            del sv["task"]
            sv["type"] = "VIP"
            sv["port"] = {"vip": self.named_vip}
            return sv
        return None


class LinkObj:
    def __init__(self, port, backend):
        self.port = port
        self.backend = backend


def task_app_types_tester():
    # Singleton counter used to uniquely define each static vip
    counter = Counter()

    agent_apps = []

    for a in ["host-mesos-id", "host-docker-id"]:
        o = AppObj(counter.count(), a)
        agent_apps.append(o)
        with util.ExampleApp("{}.json".format(a)) as c_filename:
            app = None
            with open(c_filename, 'r') as c:
                app = json.load(c)

                app['portDefinitions'][0]["labels"] = o.portlabels
                app["cmd"] = "{} {}".format(app["cmd"], o.ret_id)

            with open(c_filename, 'w') as c:
                # Wipe the file
                c.truncate(0)

                s = json.dumps(app)
                logger.info(s)
                c.write(s)
                c.flush()

                util.retry_sh("dcos marathon app add {}".format(c_filename))

    for a in ["bridge-docker-id"]:
        o = AppObj(counter.count(), a)
        agent_apps.append(o)
        with util.ExampleApp("{}.json".format(a)) as c_filename:
            app = None
            with open(c_filename, 'r') as c:
                app = json.load(c)

                app["container"]["portMappings"][0]["labels"] = o.portlabels
                app["cmd"] = "{} {}".format(app["cmd"], o.ret_id)

            with open(c_filename, 'w') as c:
                # Wipe the file
                c.truncate(0)

                s = json.dumps(app)
                logger.info(s)
                c.write(s)
                c.flush()

                util.retry_sh("dcos marathon app add {}".format(c_filename))

    container_apps = []

    for a in ["overlay-mesos-id", "overlay-docker-id",
              "overlay-host-docker-id"]:
        o = AppObj(counter.count(), a)
        container_apps.append(o)
        with util.ExampleApp("{}.json".format(a)) as c_filename:
            app = None
            with open(c_filename, 'r') as c:
                app = json.load(c)

                app["container"]["portMappings"][0]["labels"] = o.portlabels
                app["cmd"] = "{} {}".format(app["cmd"], o.ret_id)

            with open(c_filename, 'w') as c:
                # Wipe the file
                c.truncate(0)

                s = json.dumps(app)
                logger.info(s)
                c.write(s)
                c.flush()

                util.retry_sh("dcos marathon app add {}".format(c_filename))

    all_apps = agent_apps+container_apps

    poolname = "sample-minimal"
    auto_obj = LinkObj(25000, "auto")
    specified_obj = LinkObj(25001, "specified")
    staticvip_obj = LinkObj(25002, "staticvip")
    namedvip_obj = LinkObj(25003, "namedvip")
    with util.ExampleConfig("{}.yaml".format(poolname)) as c_filename:
        y = None
        with open(c_filename, 'r') as c:
            y = yaml.load(c)
            fe = [
                {
                    "bindPort": auto_obj.port,
                    "protocol": "HTTP",
                    "linkBackend": {"defaultBackend": auto_obj.backend},
                },
                {
                    "bindPort": specified_obj.port,
                    "protocol": "HTTP",
                    "linkBackend": {"defaultBackend": specified_obj.backend},
                },
                {
                    "bindPort": staticvip_obj.port,
                    "protocol": "HTTP",
                    "linkBackend": {"defaultBackend": staticvip_obj.backend},
                },
                {
                    "bindPort": namedvip_obj.port,
                    "protocol": "HTTP",
                    "linkBackend": {"defaultBackend": namedvip_obj.backend},
                }
            ]

            auto_sv = [o.backend_server(AppObj.ST_AUTO) for o in all_apps]
            specified_sv = [o.backend_server(AppObj.ST_AGENT)
                            for o in agent_apps]
            specified_sv += [o.backend_server(AppObj.ST_CONTAINER)
                             for o in container_apps]
            static_vip_sv = [o.backend_server(AppObj.ST_STATIC_VIP)
                             for o in all_apps]
            named_vip_sv = [o.backend_server(AppObj.ST_NAMED_VIP)
                            for o in all_apps]

            be = [
                {
                    "name": auto_obj.backend,
                    "protocol": "HTTP",
                    "servers": auto_sv,
                },
                {
                    "name": specified_obj.backend,
                    "protocol": "HTTP",
                    "servers": specified_sv,
                },
                {
                    "name": staticvip_obj.backend,
                    "protocol": "HTTP",
                    "servers": static_vip_sv,
                },
                {
                    "name": namedvip_obj.backend,
                    "protocol": "HTTP",
                    "servers": named_vip_sv,
                }
            ]

            y["pools"][0]["haproxy"]["frontends"] = fe
            y["pools"][0]["haproxy"]["backends"] = be
        with open(c_filename, 'w') as c:
            # Wipe the file
            c.truncate(0)

            s = yaml.dump(y)
            logger.info(s)
            c.write(s)
            c.flush()
            util.retry_sh('dcos edgelb create {}'.format(c_filename))

    edgelb.pool_up(poolname, 1)

    logger.info("waiting for auto ids")
    auto_ids = [x.ret_id for x in all_apps]
    wait_ids(poolname, auto_obj.port, auto_ids)

    logger.info("waiting for specified ids")
    specified_ids = [x.ret_id for x in all_apps]
    wait_ids(poolname, specified_obj.port, specified_ids)

    logger.info("waiting for static vip ids")
    static_vip_ids = [x.ret_id for x in all_apps]
    wait_ids(poolname, staticvip_obj.port, static_vip_ids)

    logger.info("waiting for named vip ids")
    named_vip_ids = [x.ret_id for x in all_apps]
    wait_ids(poolname, namedvip_obj.port, named_vip_ids)

    ids = [a.name for a in all_apps]
    util.kill_marathon_apps(ids)

    edgelb.delete_all_pools(True)


def pod_app_types_tester():
    # Singleton counter used to uniquely define each static vip
    counter = Counter()

    # We collect pod names because these are not 1:1 with the task names, which
    # are what are actually used for load balancing.
    pod_names = []

    agent_apps = []

    for a in ["pod-host-mesos-id"]:
        o = AppObj(counter.count(), a)
        agent_apps.append(o)
        with util.ExampleApp("{}.json".format(a)) as c_filename:
            app = None
            with open(c_filename, 'r') as c:
                app = json.load(c)

                pod_names.append(app["id"])
                app["containers"][0]["endpoints"][0]["labels"] = o.portlabels
                cmd = app["containers"][0]["exec"]["command"]["shell"]
                newcmd = "{} {}".format(cmd, o.ret_id)
                app["containers"][0]["exec"]["command"]["shell"] = newcmd

            with open(c_filename, 'w') as c:
                # Wipe the file
                c.truncate(0)

                s = json.dumps(app)
                logger.info(s)
                c.write(s)
                c.flush()

                util.retry_sh("dcos marathon pod add {}".format(c_filename))

    container_apps = []

    for a in ["pod-overlay-mesos-id"]:
        o = AppObj(counter.count(), a)
        container_apps.append(o)
        with util.ExampleApp("{}.json".format(a)) as c_filename:
            app = None
            with open(c_filename, 'r') as c:
                app = json.load(c)

                pod_names.append(app["id"])
                app["containers"][0]["endpoints"][0]["labels"] = o.portlabels
                cmd = app["containers"][0]["exec"]["command"]["shell"]
                newcmd = "{} {}".format(cmd, o.ret_id)
                app["containers"][0]["exec"]["command"]["shell"] = newcmd

            with open(c_filename, 'w') as c:
                # Wipe the file
                c.truncate(0)

                s = json.dumps(app)
                logger.info(s)
                c.write(s)
                c.flush()

                util.retry_sh("dcos marathon pod add {}".format(c_filename))

    all_apps = agent_apps+container_apps

    poolname = "sample-minimal"
    auto_obj = LinkObj(25000, "auto")
    specified_obj = LinkObj(25001, "specified")
    staticvip_obj = LinkObj(25002, "staticvip")
    namedvip_obj = LinkObj(25003, "namedvip")
    with util.ExampleConfig("{}.yaml".format(poolname)) as c_filename:
        y = None
        with open(c_filename, 'r') as c:
            y = yaml.load(c)
            fe = [
                {
                    "bindPort": auto_obj.port,
                    "protocol": "HTTP",
                    "linkBackend": {"defaultBackend": auto_obj.backend},
                },
                {
                    "bindPort": specified_obj.port,
                    "protocol": "HTTP",
                    "linkBackend": {"defaultBackend": specified_obj.backend},
                },
                {
                    "bindPort": staticvip_obj.port,
                    "protocol": "HTTP",
                    "linkBackend": {"defaultBackend": staticvip_obj.backend},
                },
                {
                    "bindPort": namedvip_obj.port,
                    "protocol": "HTTP",
                    "linkBackend": {"defaultBackend": namedvip_obj.backend},
                }
            ]

            auto_sv = [o.backend_server(AppObj.ST_AUTO) for o in all_apps]
            specified_sv = [o.backend_server(AppObj.ST_AGENT)
                            for o in agent_apps]
            specified_sv += [o.backend_server(AppObj.ST_CONTAINER)
                             for o in container_apps]
            static_vip_sv = [o.backend_server(AppObj.ST_STATIC_VIP)
                             for o in all_apps]
            named_vip_sv = [o.backend_server(AppObj.ST_NAMED_VIP)
                            for o in all_apps]

            be = [
                {
                    "name": auto_obj.backend,
                    "protocol": "HTTP",
                    "servers": auto_sv,
                },
                {
                    "name": specified_obj.backend,
                    "protocol": "HTTP",
                    "servers": specified_sv,
                },
                {
                    "name": staticvip_obj.backend,
                    "protocol": "HTTP",
                    "servers": static_vip_sv,
                },
                {
                    "name": namedvip_obj.backend,
                    "protocol": "HTTP",
                    "servers": named_vip_sv,
                }
            ]

            y["pools"][0]["haproxy"]["frontends"] = fe
            y["pools"][0]["haproxy"]["backends"] = be
        with open(c_filename, 'w') as c:
            # Wipe the file
            c.truncate(0)

            s = yaml.dump(y)
            logger.info(s)
            c.write(s)
            c.flush()
            util.retry_sh('dcos edgelb create {}'.format(c_filename))

    edgelb.pool_up(poolname, 1)

    logger.info("waiting for auto ids")
    auto_ids = [x.ret_id for x in all_apps]
    wait_ids(poolname, auto_obj.port, auto_ids)

    logger.info("waiting for specified ids")
    specified_ids = [x.ret_id for x in all_apps]
    wait_ids(poolname, specified_obj.port, specified_ids)

    logger.info("waiting for static vip ids")
    static_vip_ids = [x.ret_id for x in all_apps]
    wait_ids(poolname, staticvip_obj.port, static_vip_ids)

    logger.info("waiting for named vip ids")
    named_vip_ids = [x.ret_id for x in all_apps]
    wait_ids(poolname, namedvip_obj.port, named_vip_ids)

    util.kill_marathon_pods(pod_names)

    edgelb.delete_all_pools(True)


@retrying.retry(wait_fixed=util.DEFAULT_WAIT * 1000,
                stop_max_delay=util.DEFAULT_TIMEOUT * 1000,
                retry_on_exception=lambda x: True)
def wait_ids(poolname, port, ids):
    fmts = "dcos task exec {} bash -c 'wget -qO- http://localhost:{}/id'"
    while True:
        tids = edgelb.get_lb_tids(poolname)
        assert len(tids) == 1
        lb_task_id = tids[0]
        o = util.sh(fmts.format(lb_task_id, port), stdout=subprocess.PIPE)
        ret_id = o.stdout.decode().strip()
        logger.info("found {}".format(ret_id))
        if ret_id in ids:
            ids.remove(ret_id)
        if len(ids) == 0:
            return
        logger.info("waiting for {}".format(ids))
