import json
import os
import subprocess
import time

import retrying

import util

# import pytest
# import test_helpers
# from dcos_test_utils import marathon

LBWORKDIR = "/dcosfiles"

DEFAULT_TIMEOUT = util.DEFAULT_TIMEOUT
DEFAULT_WAIT = util.DEFAULT_WAIT

logger = util.get_logger(__name__)


class EdgeLB(object):
    def __init__(self):
        self.id = util.rand_str(12)
        self.wd = util.workdir()
        self.privkey = "{}/edge-lb-private-key-{}.pem".format(self.wd, self.id)
        self.pubkey = "{}/edge-lb-public-key-{}.pem".format(self.wd, self.id)
        self.principal = "edge-lb-principal-{}".format(self.id)
        self.secretname = "edge-lb-secret-{}".format(self.id)
        self.optionsfile = "{}/edge-lb-options-{}.json".format(
                self.wd, self.id)

    def install(self):
        # There's a strange bug where `make` commands have some messed up
        # I/O or something that prevents this from running inside python
        # subprocess.
        o = util.sh("make -n -C framework add-repos", stdout=subprocess.PIPE)
        make_stdout = o.stdout.decode()
        logger.info(make_stdout)
        for line in o.stdout.decode().splitlines():
            # The output of `make -n` changes between linux and macOS, on
            # linux it also prints the command being run in stdout.
            if line.startswith("dcos"):
                util.sh(line)

        util.sh("dcos package install dcos-enterprise-cli --yes --cli",
                timer=True)
        util.sh("dcos security org service-accounts keypair {} {}".format(
            self.privkey, self.pubkey))
        cmd = 'dcos security org service-accounts create -p {} -d "{}" {}'
        util.sh(cmd.format(self.pubkey,
                           "Edge-LB {}".format(self.id),
                           self.principal))
        util.sh("dcos security org groups add_user superusers {}".format(
            self.principal))
        cmd = "dcos security secrets create-sa-secret --strict {} {} {}"
        util.sh(cmd.format(self.privkey, self.principal, self.secretname))

        util.wait_marathon()

        with open(self.optionsfile, 'w') as ofile:
            data = {
                    'service': {
                        'secretName': self.secretname,
                        'principal': self.principal,
                        'cpus': 0.3,
                        'mem': 300,
                        'forcePull': True
                        }
                    }
            json.dump(data, ofile)
        util.sh("dcos package install --options={} edgelb --yes".format(
            self.optionsfile), timer=True)
        util.sh("dcos package install edgelb-pool --cli --yes", timer=True)

        self.ping()
        time.sleep(3)

    @retrying.retry(wait_fixed=DEFAULT_WAIT * 1000,
                    stop_max_delay=DEFAULT_TIMEOUT * 1000,
                    retry_on_exception=lambda x: True)
    def ping(self):
        util.sh("dcos edgelb ping")

    def uninstall(self, ignore_error=False):
        check = (not ignore_error)

        delete_all_pools(ignore_error=ignore_error)

        util.sh("dcos package uninstall edgelb --yes", check=check)
        util.sh("dcos package uninstall edgelb-pool --cli", check=check)

        util.sh("dcos security secrets delete {}".format(self.secretname),
                check=check)
        util.sh("dcos security org service-accounts delete {}".format(
            self.principal), check=check)

        try:
            util.uninstall_repos()
        except Exception as e:
            if not ignore_error:
                raise e
            logger.info(e)
        try:
            for f in [self.privkey, self.pubkey, self.optionsfile]:
                os.remove(f)
        except Exception as e:
            if not ignore_error:
                raise e
            logger.info(e)
        util.wait_marathon()


def delete_all_pools(ignore_error=False):
    logger.info('deleting all pools')
    poolnames = list_pools(ignore_error=ignore_error)
    logger.info('deleting all pools, poolnames: {}'.format(poolnames))
    for poolname in poolnames:
        delete_pool(poolname)


def force_clean_pools(ignore_error=False):
    check = (not ignore_error)

    util.sh("dcos package uninstall edgelb-pool --yes", check=check)


@retrying.retry(wait_fixed=DEFAULT_WAIT * 1000,
                stop_max_delay=DEFAULT_TIMEOUT * 1000,
                retry_on_exception=lambda x: True)
def pool_up(poolname, num_instances):
    """
    Wait for a pool to come up
    """
    tids = get_lb_tids(poolname)
    assert len(tids) == num_instances
    for tid in tids:
        pool_server_healthy(tid)


@retrying.retry(wait_fixed=DEFAULT_WAIT * 1000,
                stop_max_delay=DEFAULT_TIMEOUT * 1000,
                retry_on_exception=lambda x: True)
def get_lb_tids(poolname):
    o = util.sh("dcos edgelb status {} --task-ids --json".format(poolname),
                stdout=subprocess.PIPE)
    tids = json.loads(o.stdout.decode())
    return tids


def pool_server_healthy(tid):
    logger.info("waiting for pool server healthy")

    # The reason we have out own way to telling success from the stdout
    # is because `dcos task exec` doesn't seem to pass on the exitcode properly
    successmsg = "pool_healthy_success_yay"
    healthcheck = ("{}/haproxy/bin/lbmgr healthcheck && "
                   "echo {}".format(LBWORKDIR, successmsg))
    o = util.sh("dcos task exec {} bash -c '{}'".format(tid, healthcheck),
                stdout=subprocess.PIPE)
    lastmsg = o.stdout.decode().strip().split()[-1]
    assert lastmsg == successmsg


def pool_task_name(poolname):
    return "dcos-edgelb_pools_{}".format(poolname)


def delete_pool(poolname):
    logger.info('deleting pool {}'.format(poolname))
    lbtids = get_lb_tids(poolname)
    logger.info('deleting pool {}, tids: {}'.format(poolname, lbtids))
    delete_pool_cmd(poolname)
    for tid in lbtids:
        util.task_down(tid)
        logger.info('deleting pool {}, tid {} is down'.format(poolname, tid))
    pool_task = pool_task_name(poolname)
    logger.info('deleting pool {}, waiting for task {} to be down'.
                format(poolname, pool_task))
    util.task_down(pool_task, exact=False)
    logger.info('deleted pool {}, task {} is down'.format(poolname, pool_task))


@retrying.retry(wait_fixed=DEFAULT_WAIT * 1000,
                stop_max_delay=DEFAULT_TIMEOUT * 1000,
                retry_on_exception=lambda x: True)
def delete_pool_cmd(poolname):
    logger.info("delete pool cmd")
    util.sh("dcos edgelb delete {}".format(poolname))


@retrying.retry(wait_fixed=DEFAULT_WAIT * 1000,
                stop_max_delay=DEFAULT_TIMEOUT * 1000,
                retry_on_exception=lambda x: True)
def list_pools(ignore_error=False):
    logger.info("list pools")
    check = (not ignore_error)
    try:
        o = util.sh("dcos edgelb list --json",
                    check=check, stdout=subprocess.PIPE)
        pools = json.loads(o.stdout.decode())
        names = []
        for p in pools:
            names.append(p['name'])
        return names
    except Exception as e:
        logger.info('list pools error: {}'.format(e))
        if ignore_error:
            return []
        raise e
